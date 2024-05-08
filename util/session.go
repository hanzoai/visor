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

package util

import (
	"sync"

	"github.com/casvisor/casvisor/util/guacamole"
	"github.com/casvisor/casvisor/util/term"

	"github.com/gorilla/websocket"
)

type GlobalSession struct {
	Id          string
	Protocol    string
	Mode        string
	WebSocket   *websocket.Conn
	GuacdTunnel *guacamole.Tunnel
	Terminal    *term.Terminal
	Observer    *Manager
	mutex       sync.Mutex

	Uptime   int64
	Hostname string
}

func (s *GlobalSession) WriteString(str string) error {
	if s.WebSocket == nil {
		return nil
	}
	defer s.mutex.Unlock()
	s.mutex.Lock()
	message := []byte(str)
	return s.WebSocket.WriteMessage(websocket.TextMessage, message)
}

func (s *GlobalSession) Close() {
	if s.GuacdTunnel != nil {
		_ = s.GuacdTunnel.Close()
	}

	if s.WebSocket != nil {
		_ = s.WebSocket.Close()
	}

	if s.Terminal != nil {
		_ = s.Terminal.Close()
	}
}

type Manager struct {
	id       string
	sessions sync.Map
}

func NewManager() *Manager {
	return &Manager{}
}

func NewObserver(id string) *Manager {
	return &Manager{
		id: id,
	}
}

func (m *Manager) Get(id string) *GlobalSession {
	value, ok := m.sessions.Load(id)
	if ok {
		return value.(*GlobalSession)
	}
	return nil
}

func (m *Manager) Add(s *GlobalSession) {
	m.sessions.Store(s.Id, s)
}

func (m *Manager) Delete(id string) {
	session := m.Get(id)
	if session != nil {
		session.Close()
		if session.Observer != nil {
			session.Observer.Clear()
		}
	}
	m.sessions.Delete(id)
}

func (m *Manager) Clear() {
	m.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*GlobalSession); ok {
			session.Close()
		}
		m.sessions.Delete(key)
		return true
	})
}

func (m *Manager) Range(f func(key string, value *GlobalSession)) {
	m.sessions.Range(func(key, value interface{}) bool {
		if session, ok := value.(*GlobalSession); ok {
			f(key.(string), session)
		}
		return true
	})
}

var GlobalSessionManager *Manager

func init() {
	GlobalSessionManager = NewManager()
}
