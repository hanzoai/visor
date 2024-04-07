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

package proxy

import (
	"github.com/casvisor/casvisor/conf"
	"github.com/casvisor/casvisor/object"
	"github.com/casvisor/casvisor/proxy/client"
	"github.com/casvisor/casvisor/proxy/server"
	"github.com/casvisor/casvisor/proxy/tunnel"
	"github.com/casvisor/casvisor/util"
)

const (
	clientMode = iota
	serverMode
)

type Starter struct {
	mode   int
	Server *server.Server
	Client *client.Client

	gatewayPort int
	gatewayHost string

	restartClientChan chan string
	startClientChan   chan string
}

func NewStarter(restartClientChan chan string) *Starter {
	if conf.GatewayAddr == nil {
		return nil
	}

	starter := &Starter{
		Client:            nil,
		Server:            nil,
		gatewayPort:       conf.GatewayAddr.Port,
		gatewayHost:       conf.GatewayAddr.IP.String(),
		restartClientChan: restartClientChan,
		startClientChan:   make(chan string, 1),
	}

	asset, err := object.GetAsset(util.GetIdFromOwnerAndName(conf.GetConfigString("casdoorOrganization"), util.GetHostname()))
	if err != nil {
		panic(err)
	}

	if asset != nil {
		starter.mode = clientMode
	} else {
		starter.mode = serverMode
	}

	return starter
}

func (s *Starter) Start() {
	if s == nil {
		return
	}

	s.initServer()
	go s.Server.Start(s.restartClientChan)

	if s.mode == clientMode {
		s.initClient()
		go s.Client.Start(s.startClientChan)
	}

	go func() {
		for {
			select {
			case <-s.startClientChan:
				s.RestartClient()
			}
		}
	}()
}

func (s *Starter) initServer() {
	proxyServer, err := server.NewProxyServer("Casvisor Proxy Server", s.gatewayPort)
	if err != nil {
		panic(err)
	}
	s.Server = proxyServer
}

func (s *Starter) initClient() {
	asset, err := object.GetAsset(util.GetIdFromOwnerAndName(conf.GetConfigString("casdoorOrganization"), util.GetHostname()))
	if err != nil {
		panic(err)
	}
	if asset == nil {
		panic("asset is nil")
	}

	proxyClient := client.NewClient(asset.Name,
		s.gatewayHost,
		s.gatewayPort,
		tunnel.AssetToAppInfo(asset),
	)
	s.Client = proxyClient
}

func (s *Starter) RestartClient() {
	s.initClient()
	go s.Client.Start(s.startClientChan)
}
