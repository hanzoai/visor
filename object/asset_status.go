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
	"github.com/casvisor/casvisor/conf"
	"github.com/casvisor/casvisor/metric"
	"github.com/casvisor/casvisor/util"
	"xorm.io/core"
)

func RunRefreshAssetStatus() error {
	assets := []*Asset{}
	err := adapter.Engine.Where("category = ?", "Machine").Find(&assets)
	if err != nil {
		return err
	}

	runner := metric.Runner{}
	for _, asset := range assets {
		// use a copy of asset to avoid data race
		a := asset
		runner.Add(func() error {
			return RefreshAssetStatus(a)
		})
	}
	runner.Wait()
	return nil
}

func RefreshAssetStatus(asset *Asset) error {
	var err error
	var status string
	var sshStatus string
	var agentStatus string

	portOpen := isPortOpen(asset.Endpoint, asset.Port)
	if portOpen {
		status = AssetStatusRunning

		if asset.Type == "SSH" {
			sshStatus = AssetStatusRunning
		}
	} else {
		status = AssetStatusStopped
		sshStatus = AssetStatusStopped
	}

	if portOpen && asset.EnableSsh {
		portOpen = isPortOpen(asset.Endpoint, asset.SshPort)
		if portOpen {
			sshStatus = AssetStatusRunning
		} else {
			sshStatus = AssetStatusStopped
		}
	}

	portOpen = isPortOpen(asset.Endpoint, util.ParseInt(conf.GetConfigString("httpport")))
	if portOpen {
		agentStatus = AssetStatusRunning
	} else {
		agentStatus = AssetStatusStopped
	}

	asset.Status = status
	asset.SshStatus = sshStatus
	asset.AgentStatus = agentStatus

	_, err = adapter.Engine.ID(core.PK{asset.Owner, asset.Name}).Cols("status", "ssh_status", "agent_status").Update(asset)
	if err != nil {
		return err
	}

	return nil
}
