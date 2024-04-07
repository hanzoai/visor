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

package client

import (
	"errors"
	"fmt"
	"time"

	"github.com/casvisor/casvisor/proxy/tunnel"
)

type Client struct {
	Name          string
	LocalPort     int
	RemoteAddr    string
	RemotePort    int
	proxyApps     map[string]*tunnel.AppInfo
	heartbeatChan chan *tunnel.Message // when get heartbeat msg, put msg in
	conn          *tunnel.Conn
}

func NewClient(name string, remoteAddr string, remotePort int, appInfo *tunnel.AppInfo) *Client {
	client := &Client{
		Name:          name,
		RemoteAddr:    remoteAddr,
		RemotePort:    remotePort,
		heartbeatChan: make(chan *tunnel.Message, 1),
		proxyApps:     make(map[string]*tunnel.AppInfo),
	}

	if appInfo != nil {
		client.proxyApps[appInfo.Name] = appInfo
	}
	return client
}

func (c *Client) sendInitAppMsg() error {
	if c.proxyApps == nil {
		return errors.New("proxyApps is nil")
	}

	msg := tunnel.NewMessage(tunnel.InitApp, "", c.Name, c.proxyApps)
	if err := c.conn.SendMessage(msg); err != nil {
		return err
	}
	return nil
}

func (c *Client) storeAppServer(msg *tunnel.Message) {
	println("---------- Sever ----------")
	for name, app := range c.proxyApps {
		fmt.Printf("[%s]:\t%s:%d\n", name, c.conn.GetRemoteIP(), app.ListenPort)
	}
	println("---------------------------")

	// prepared, start first heartbeat
	c.heartbeatChan <- msg

	// keep Heartbeat
	go func() {
		for {
			select {
			case <-c.heartbeatChan:
				time.Sleep(tunnel.HeartbeatInterval)
				resp := tunnel.NewMessage(tunnel.ClientHeartbeat, "", c.Name, nil)
				err := c.conn.SendMessage(resp)
				if err != nil {
					return
				}
			case <-time.After(tunnel.HeartbeatTimeout):
				if c.conn != nil {
					c.conn.Close()
					return
				}
			}
		}
	}()
}

func (c *Client) handleBindMsg(msg *tunnel.Message) {
	appProxyName := msg.Content
	if appProxyName == "" {
		return
	}

	appInfo, ok := c.proxyApps[appProxyName]
	if !ok {
		return
	}

	localConn, err := tunnel.Dial(appInfo.LocalAddress, appInfo.LocalPort)
	if err != nil {
		defer localConn.Close()
		panic(err)
		return
	}

	remoteConn, err := tunnel.Dial(c.RemoteAddr, appInfo.ListenPort)
	if err != nil {
		defer remoteConn.Close()
		panic(err)
		return
	}

	bindMsg := tunnel.NewMessage(tunnel.ClientBind, msg.Content, c.Name, nil)
	err = remoteConn.SendMessage(bindMsg)
	if err != nil {
		panic(err)
		return
	}

	go tunnel.Bind(localConn, remoteConn)
}

func (c *Client) Start(startClientChan chan string) {
	conn, err := tunnel.Dial(c.RemoteAddr, c.RemotePort)
	if err != nil {
		return
	}
	c.conn = conn
	defer c.conn.Close()

	err = c.sendInitAppMsg()
	if err != nil {
		return
	}

	for {
		msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		switch msg.Type {
		case tunnel.ServerHeartbeat:
			c.heartbeatChan <- msg
		case tunnel.AppMsg:
			c.storeAppServer(msg)
		case tunnel.AppWaitBind:
			go c.handleBindMsg(msg)
		case tunnel.ClientRestart:
			c.conn.Close()
			startClientChan <- msg.Name
			return
		}
	}
}
