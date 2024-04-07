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

package tunnel

import (
	"time"

	"github.com/casvisor/casvisor/object"
)

const (
	HeartbeatInterval = 10 * time.Second
	HeartbeatTimeout  = 30 * time.Second
	BindConnTimeout   = 10 * time.Second

	RetryTimes = 5
)

type AppInfo struct {
	Name         string
	ListenPort   int    // used in server
	LocalAddress string // used in client
	LocalPort    int    // used in client
}

func AssetToAppInfo(asset *object.Asset) *AppInfo {
	return &AppInfo{
		Name:         asset.Name,
		ListenPort:   asset.GatewayPort,
		LocalAddress: asset.Endpoint,
		LocalPort:    asset.Port,
	}
}
