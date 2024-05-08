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

package term

import (
	"golang.org/x/crypto/ssh"

	"github.com/pkg/sftp"
)

type Terminal struct {
	SshClient  *ssh.Client
	SftpClient *sftp.Client
}

func NewTerminal(addr, username, password string) (*Terminal, error) {
	sshClient, err := NewSshClient(addr, username, password)
	if err != nil {
		return nil, err
	}

	terminal := Terminal{
		SshClient: sshClient,
	}

	return &terminal, nil
}

func (t *Terminal) Close() error {
	if t.SftpClient != nil {
		err := t.SftpClient.Close()
		if err != nil {
			return err
		}
	}

	if t.SshClient != nil {
		err := t.SshClient.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
