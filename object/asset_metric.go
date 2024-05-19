// Copyright 2024 The casbin Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package object

import (
	"fmt"
	"sync"

	"github.com/casvisor/casvisor/metric"
	"github.com/casvisor/casvisor/util/term"
	"golang.org/x/crypto/ssh"
	"xorm.io/core"
)

var (
	sshClients      sync.Map
	sshClientsMutex sync.RWMutex
)

func RunUpdateAssetMetrics() {
	assets := []*Asset{}
	err := adapter.Engine.Where("enable_ssh = ? or type = ?", true, "SSH").Find(&assets)
	if err != nil {
		return
	}

	runner := metric.Runner{}
	for _, asset := range assets {
		// use a copy of asset to avoid data race
		a := asset
		runner.Add(func() error {
			return UpdateAssetMetric(a)
		})
	}
	runner.Wait()
}

func UpdateAssetMetric(asset *Asset) error {
	client, err := GetSshClient(asset)
	if err != nil {
		asset.IsActive = false
		return fmt.Errorf("%s: %s", asset.GetId(), err.Error())
	}
	asset.IsActive = true

	stat := &metric.Stat{}
	stat, err = metric.GetAllStat(client, stat)
	if err != nil {
		return err
	}

	LoadAssetStat(asset, stat)
	_, err = adapter.Engine.ID(core.PK{asset.Owner, asset.Name}).AllCols().Update(asset)
	if err != nil {
		return err
	}

	return err
}

func GetSshClient(asset *Asset) (*ssh.Client, error) {
	sshClientsMutex.RLock()
	if client, ok := sshClients.Load(asset.GetId()); ok {
		sshClientsMutex.RUnlock()
		return client.(*ssh.Client), nil
	}
	sshClientsMutex.RUnlock()

	sshClientsMutex.Lock()
	defer sshClientsMutex.Unlock()

	client, err := term.NewSshClient(asset.GetAddr(), asset.Username, asset.Password)
	if err != nil {
		return nil, err
	}

	sshClients.Store(asset.GetId(), client)
	return client, nil
}

func LoadAssetStat(asset *Asset, stat *metric.Stat) {
	if len(stat.FsInfos) > 0 {
		fsInfo := stat.FsInfos[0]
		asset.DiskTotal = int64(fsInfo.Used + fsInfo.Free)
		asset.DiskCurrent = int64(fsInfo.Used)
	}

	asset.MemTotal = stat.MemTotal
	asset.MemCurrent = stat.MemTotal - stat.MemAvailable
	asset.CpuTotal = stat.Cpu.CoreNum
	asset.CpuCurrent = 100 - stat.Cpu.Idle
}

func CloseSshClients() {
	sshClientsMutex.Lock()
	defer sshClientsMutex.Unlock()

	sshClients.Range(func(key, value interface{}) bool {
		client := value.(*ssh.Client)
		client.Close()
		sshClients.Delete(key)
		return true
	})

	metric.CleanupPreCpuMap()
}
