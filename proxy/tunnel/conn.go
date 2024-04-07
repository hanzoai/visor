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
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/casvisor/casvisor/util"
)

const (
	BufferEndFlag = '\n'
)

type Conn struct {
	net.Conn
	Reader    *bufio.Reader
	closeFlag bool
	Id        string
}

func NewConn(tcpConn net.Conn) *Conn {
	c := &Conn{
		Conn:      tcpConn,
		closeFlag: false,
		Reader:    bufio.NewReader(tcpConn),
		Id:        util.GenerateId(),
	}
	return c
}

func (c *Conn) String() string {
	return fmt.Sprintf("%s <===> %s", c.GetLocalAddr(), c.GetRemoteAddr())
}

func (c *Conn) Close() {
	if c.Conn != nil && c.closeFlag == false {
		c.closeFlag = true
		c.Conn.Close()
	}
}

func (c *Conn) IsClosed() bool {
	return c.closeFlag
}

func (c *Conn) GetRemoteAddr() string {
	return c.RemoteAddr().String()
}

func (c *Conn) GetRemoteIP() string {
	return strings.Split(c.GetRemoteAddr(), ":")[0]
}

func (c *Conn) GetLocalAddr() string {
	return c.LocalAddr().String()
}

func (c *Conn) Send(buff []byte) error {
	buffer := bytes.NewBuffer(buff)
	buffer.WriteByte(BufferEndFlag)

	_, err := c.Conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func (c *Conn) SendMessage(msg *Message) error {
	msgBytes, _ := json.Marshal(msg)
	err := c.Send(msgBytes)
	if err != nil {
		return err
	}
	return nil
}

func (c *Conn) Read() ([]byte, error) {
	buff, err := c.Reader.ReadBytes(BufferEndFlag)
	if err == io.EOF {
		c.Close()
	}
	return buff, err
}

func (c *Conn) ReadMessage() (*Message, error) {
	msgBytes, err := c.Read()
	if err != nil {
		return nil, err
	}

	message := &Message{}
	if err = json.Unmarshal(msgBytes, message); err != nil {
		return nil, err
	}

	return message, nil
}

// Bind will block until connection close
func Bind(c1 *Conn, c2 *Conn) {
	var wait sync.WaitGroup
	pipe := func(to *Conn, from *Conn) {
		defer to.Close()
		defer from.Close()
		defer wait.Done()

		var err error
		_, err = io.Copy(to.Conn, from.Conn)
		if err != nil {
			return
		}
	}

	wait.Add(2)
	go pipe(c1, c2)
	go pipe(c2, c1)
	wait.Wait()
	return
}

func Dial(remoteAddr string, remotePort int) (*Conn, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", remoteAddr, remotePort))
	if err != nil {
		return nil, err
	}

	remoteConn := NewConn(conn)
	return remoteConn, nil
}
