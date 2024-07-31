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

package server

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casvisor/casvisor/conf"
	"github.com/casvisor/casvisor/object"
	"github.com/casvisor/casvisor/proxy/tunnel"
	"github.com/casvisor/casvisor/util"
)

const (
	Idle = iota
	Ready
	Work
)

// Server commonServer: Used to establish connection and keep heartbeat。
// appServer(appProxyServer): Used to proxy data
type Server struct {
	Name       string
	listenPort int

	listener          *tunnel.Listener
	status            int                  // used in appServer only
	heartbeatChan     chan *tunnel.Message // when get heartbeat msg, put msg in, used in commonServer only
	RestartClientChan chan string

	proxyServer map[string]*Server      // appServer which is listening its own port, used in commonServer only
	clientConn  map[string]*tunnel.Conn // used in commonServer only
	userConnMap sync.Map                // map[appServerName]UserConn, used in appServer only
}

func NewProxyServer(name string, listenPort int) (*Server, error) {
	listener, err := tunnel.NewListener(listenPort)
	if err != nil {
		return nil, err
	}

	server := &Server{
		Name:          name,
		listenPort:    listenPort,
		status:        Idle,
		listener:      listener,
		proxyServer:   make(map[string]*Server),
		clientConn:    make(map[string]*tunnel.Conn),
		heartbeatChan: make(chan *tunnel.Message, 1),
	}
	return server, nil
}

func (s *Server) CloseClient(clientConn *tunnel.Conn) {
	logs.Info("close conn: ", clientConn.String())
	clientConn.Close()
	for _, app := range s.proxyServer {
		app.listener.Close()
	}

	s.proxyServer = make(map[string]*Server, len(s.proxyServer))
}

func (s *Server) GetProxyApps(msg *tunnel.Message) (map[string]*tunnel.AppInfo, error) {
	if msg.Meta == nil {
		return nil, errors.New("has no app to proxy")
	}

	proxyApps := make(map[string]*tunnel.AppInfo)
	for name, app := range msg.Meta.(map[string]interface{}) {
		App := app.(map[string]interface{})

		proxyApps[name] = &tunnel.AppInfo{
			Name:       App["Name"].(string),
			ListenPort: int(App["ListenPort"].(float64)),
			LocalPort:  int(App["LocalPort"].(float64)),
		}
	}
	return proxyApps, nil
}

func (s *Server) initApp(clientConn *tunnel.Conn, msg *tunnel.Message) {
	proxyApps, err := s.GetProxyApps(msg)
	if err != nil {
		logs.Error(err)
		s.CloseClient(clientConn)
		return
	}

	for _, app := range proxyApps {
		go s.startProxyApp(clientConn, app)
	}

	// send app info to client
	resp := tunnel.NewMessage(tunnel.AppMsg, "", s.Name, proxyApps)
	err = clientConn.SendMessage(resp)
	if err != nil {
		logs.Error("send app info to client failed: ", err)
		s.CloseClient(clientConn)
		return
	}

	for _, app := range proxyApps {
		assetId := util.GetIdFromOwnerAndName(conf.GetConfigString("casdoorOrganization"), app.Name)
		asset, err := object.GetAsset(assetId)
		if err != nil {
			logs.Error(err)
		}

		asset.Status = object.AssetStatusRunning
		_, err = object.UpdateAsset(assetId, asset)
		if err != nil {
			logs.Error(err)
		}
	}

	// keep Heartbeat
	go func() {
		for {
			select {
			case <-s.heartbeatChan:
				resp := tunnel.NewMessage(tunnel.ServerHeartbeat, "", s.Name, nil)
				err := clientConn.SendMessage(resp)
				if err != nil {
					s.CloseClient(clientConn)
					return
				}
			case <-time.After(tunnel.HeartbeatTimeout):
				if clientConn != nil {
					s.CloseClient(clientConn)
				}
				return
			}
		}
	}()
}

func (s *Server) startProxyApp(clientConn *tunnel.Conn, app *tunnel.AppInfo) {
	if ps, ok := s.proxyServer[app.Name]; ok {
		logs.Info("app server name", ps.Name)
		ps.listener.Close()
		ps.listener.Stop()
	}

	appProxyServer, err := NewProxyServer(app.Name, app.ListenPort)
	if err != nil {
		logs.Error(err)
		return
	}
	s.proxyServer[app.Name] = appProxyServer
	s.clientConn[app.Name] = clientConn

	for {
		conn, err := appProxyServer.listener.GetConn()
		if err != nil {
			logs.Error("appProxyServer get conn err:", err)
			return
		}
		logs.Info("user connect success:", conn.String())

		// connection from client
		if appProxyServer.status == Ready && conn.GetRemoteIP() == clientConn.GetRemoteIP() {
			msg, err := conn.ReadMessage()
			if err != nil {
				logs.Warn("proxy client read err:", err)
				if err == io.EOF {
					logs.Error("Name [%s], server is dead!", appProxyServer.Name)
					s.CloseClient(conn)
					return
				}
				continue
			}
			if msg.Type != tunnel.ClientBind {
				logs.Warn("get wrong msg")
				continue
			}

			appName := msg.Content
			newClientConn, ok := s.userConnMap.Load(appName)
			if !ok {
				logs.Error("userConnMap load failed. appProxyAddrEny:", appName)
				continue
			}
			s.userConnMap.Delete(appName)

			waitToJoinClientConn := conn
			waitToJoinUserConn := newClientConn.(*tunnel.Conn)
			go tunnel.Bind(waitToJoinUserConn, waitToJoinClientConn)
			appProxyServer.status = Work
		} else {
			// connection from user
			s.userConnMap.Store(app.Name, conn)
			time.AfterFunc(tunnel.BindConnTimeout, func() {
				uc, ok := s.userConnMap.Load(app.Name)
				if !ok || uc == nil {
					return
				}
				if conn == uc.(*tunnel.Conn) {
					logs.Error("Name [%s], user conn [%s], join connections timeout", s.Name, conn.GetRemoteAddr())
					conn.Close()
				}
				appProxyServer.status = Idle
			})

			// notify client to connect
			msg := tunnel.NewMessage(tunnel.AppWaitBind, app.Name, app.Name, nil)
			err := clientConn.SendMessage(msg)
			if err != nil {
				logs.Warn(err)
				return
			}
			appProxyServer.status = Ready
		}
	}
}

func (s *Server) Start(clientRestartChan chan string) {
	s.RestartClientChan = clientRestartChan
	go s.handleRestartChan()

	for {
		clientConn, err := s.listener.GetConn()
		if err != nil {
			logs.Warn("proxy get conn err:", err.Error())
			continue
		}

		go s.handleConn(clientConn)
	}
}

func (s *Server) handleConn(clientConn *tunnel.Conn) {
	for {
		msg, err := clientConn.ReadMessage()
		if err != nil {
			if err == io.EOF {
				logs.Info("Name [%s], client is dead!", s.Name)
				s.CloseClient(clientConn)
			}
			return
		}

		switch msg.Type {
		case tunnel.InitApp:
			go s.initApp(clientConn, msg)
		case tunnel.ClientHeartbeat:
			s.heartbeatChan <- msg
		}
	}
}

func (s *Server) handleRestartChan() {
	for {
		appName := <-s.RestartClientChan

		if clientConn, ok := s.clientConn[appName]; ok {
			msg := tunnel.NewMessage(tunnel.ClientRestart, appName, appName, nil)
			err := clientConn.SendMessage(msg)
			if err != nil {
				logs.Error("send restart msg to client failed: ", err)
				continue
			}
			s.proxyServer[appName].CloseClient(clientConn)
		}
	}
}
