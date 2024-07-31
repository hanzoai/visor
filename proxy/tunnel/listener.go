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
	"fmt"
	"net"
	"sync"

	"github.com/beego/beego/logs"
	"github.com/sheerun/queue"
)

type Listener struct {
	net.Listener
	connChan chan *Conn
	ConnList *queue.Queue
	once     sync.Once
}

func (l *Listener) Close() {
	l.once.Do(l.close)
}

func (l *Listener) close() {
	for l.ConnList.Length() != 0 {
		if c := l.ConnList.Pop(); c != nil {
			if conn := c.(*Conn); !conn.IsClosed() {
				conn.Close()
			}
		}
	}
}

func (l *Listener) StartListen() {
	if l.Listener == nil {
		logs.Error("tcp listener is nil")
	}
	logs.Info("start listen :", l.Addr())
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		logs.Info("get remote conn: %s -> %s", conn.RemoteAddr(), conn.LocalAddr())
		c := NewConn(conn)
		l.connChan <- c
	}
}

func NewListener(port int) (*Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		Listener: ln,
		connChan: make(chan *Conn, 1),
		ConnList: queue.New(),
	}
	go listener.StartListen()
	return listener, nil
}

// GetConn wait util get one new connection or listener is closed
// if listener is closed, err returned
func (l *Listener) GetConn() (conn *Conn, err error) {
	var ok bool
	conn, ok = <-l.connChan
	if !ok {
		return conn, fmt.Errorf("channel close")
	}
	l.ConnList.Append(conn)
	return conn, nil
}

func (l *Listener) Stop() {
	l.Listener.Close()
}
