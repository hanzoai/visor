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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casvisor/casvisor/conf"
	"github.com/casvisor/casvisor/metric"
	"github.com/casvisor/casvisor/util/term"
	"golang.org/x/crypto/ssh"
	"xorm.io/core"
)

var (
	sshClients      sync.Map
	sshClientsMutex sync.RWMutex
	httpClient      *http.Client
)

func init() {
	httpClient = newHTTPClient()
}

func RunUpdateAssetMetrics() {
	assets := []*Asset{}
	err := adapter.Engine.Where("ssh_status = ? or agent_status = ? ", AssetStatusRunning, AssetStatusRunning).Find(&assets)
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
	if asset.AgentStatus == AssetStatusRunning {
		systemInfo, err := getMetrics(asset)
		if err != nil {
			return err
		}

		asset.DiskTotal = int64(systemInfo.DiskTotal)
		asset.DiskCurrent = int64(systemInfo.DiskUsed)
		asset.MemTotal = int64(systemInfo.MemoryTotal)
		asset.MemCurrent = int64(systemInfo.MemoryUsed)
		asset.CpuTotal = len(systemInfo.CpuUsage)
		asset.CpuCurrent = float32(systemInfo.CpuUsage[0])
	} else if asset.SshStatus == AssetStatusRunning {
		client, err := GetSshClient(asset)
		if err != nil {
			return fmt.Errorf("%s: %s", asset.GetId(), err.Error())
		}

		stat := &metric.Stat{}
		stat, err = metric.GetAllStat(client, stat)
		if err != nil {
			return err
		}

		LoadAssetStat(asset, stat)
	}
	_, err := adapter.Engine.ID(core.PK{asset.Owner, asset.Name}).Cols("disk_total", "disk_current", "mem_total", "mem_current", "cpu_total", "cpu_current").Update(asset)
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

	client, err := term.NewSshClient(asset.GetSshAddr(), asset.Username, asset.Password)
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

func newHTTPClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
}

func getMetrics(asset *Asset) (*SystemInfo, error) {
	url := fmt.Sprintf("http://%s:%s/agent/get-system-info", asset.Endpoint, conf.GetConfigString("httpport"))

	casdoorsdk.SetHttpClient(httpClient)
	res, err := casdoorsdk.DoGetBytes(url)
	if err != nil {
		return nil, err
	}

	var systemInfo *SystemInfo
	err = json.Unmarshal(res, &systemInfo)
	if err != nil {
		return nil, err
	}

	return systemInfo, nil
}
