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

package dbgate

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
)

type Connection struct {
	Id              string `json:"_id,"`
	Engine          string `json:"engine,omitempty"`
	Server          string `json:"server,omitempty"`
	User            string `json:"user,omitempty"`
	Password        string `json:"password,omitempty"`
	PasswordMode    string `json:"passwordMode,omitempty"`
	Port            int    `json:"port,omitempty"`
	DatabaseUrl     string `json:"databaseUrl,omitempty"`
	UseDatabaseUrl  bool   `json:"useDatabaseUrl,omitempty"`
	DatabaseFile    string `json:"databaseFile,omitempty"`
	SocketPath      string `json:"socketPath,omitempty"`
	AuthType        string `json:"authType,omitempty"`
	DefaultDatabase string `json:"defaultDatabase,omitempty"`
	SingleDatabase  bool   `json:"singleDatabase,omitempty"`
	DisplayName     string `json:"displayName,omitempty"`
	IsReadOnly      bool   `json:"isReadOnly,omitempty"`
	Databases       string `json:"databases,omitempty"`
	Parent          string `json:"parent,omitempty"`

	UseSshTunnel       bool   `json:"useSshTunnel,omitempty"`
	SshHost            string `json:"sshHost,omitempty"`
	SshPort            string `json:"sshPort,omitempty"`
	SshMode            string `json:"sshMode,omitempty"`
	SshLogin           string `json:"sshLogin,omitempty"`
	SshPassword        string `json:"sshPassword,omitempty"`
	SshKeyfile         string `json:"sshKeyfile,omitempty"`
	SshKeyfilePassword string `json:"sshKeyfilePassword,omitempty"`
	UseSsl             bool   `json:"useSsl,omitempty"`

	SslCaFile             string `json:"sslCaFile,omitempty"`
	SslCertFile           string `json:"sslCertFile,omitempty"`
	SslCertFilePassword   string `json:"sslCertFilePassword,omitempty"`
	SslKeyFile            string `json:"sslKeyFile,omitempty"`
	SslRejectUnauthorized bool   `json:"sslRejectUnauthorized,omitempty"`
}

type JsonLinesDatabase struct {
	filename      string
	data          []*Connection
	loadedOk      bool
	loadPerformed bool
	mu            sync.Mutex
}

func NewConnectionDataStore() *JsonLinesDatabase {
	return NewJsonLinesDatabase(dataDir() + "/connections.jsonl")
}

func NewJsonLinesDatabase(filename string) *JsonLinesDatabase {
	return &JsonLinesDatabase{
		filename: filename,
		data:     make([]*Connection, 0),
	}
}

func (db *JsonLinesDatabase) save() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if !db.loadedOk {
		return nil
	}

	file, err := os.Create(db.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, item := range db.data {
		jsonData, err := json.Marshal(item)
		if err != nil {
			return err
		}
		file.WriteString(string(jsonData) + "\n")
	}

	return nil
}

func (db *JsonLinesDatabase) ensureLoaded() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if !db.loadPerformed {
		if _, err := os.Stat(db.filename); os.IsNotExist(err) {
			db.loadedOk = true
			db.loadPerformed = true
			return nil
		}

		file, err := os.Open(db.filename)
		if err != nil {
			return err
		}
		defer file.Close()

		bytes, err := os.ReadFile(db.filename)
		if err != nil {
			return err
		}

		lines := strings.Split(string(bytes), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				item := &Connection{}
				err := json.Unmarshal([]byte(line), item)
				if err != nil {
					return err
				}
				db.data = append(db.data, item)
			}
		}

		db.loadedOk = true
		db.loadPerformed = true
	}

	return nil
}

func (db *JsonLinesDatabase) Insert(connection *Connection) error {
	if err := db.ensureLoaded(); err != nil {
		return err
	}

	for _, item := range db.data {
		if item.Id == connection.Id {
			return errors.New("Cannot insert duplicate ID into " + db.filename)
		}
	}

	db.data = append(db.data, connection)
	if err := db.save(); err != nil {
		return err
	}

	return nil
}

func (db *JsonLinesDatabase) Get(id string) (*Connection, error) {
	if err := db.ensureLoaded(); err != nil {
		return nil, err
	}

	for _, item := range db.data {
		if item.Id == id {
			return item, nil
		}
	}

	return nil, nil
}

func (db *JsonLinesDatabase) Update(connection *Connection) error {
	if err := db.ensureLoaded(); err != nil {
		return err
	}

	for i, item := range db.data {
		if item.Id == connection.Id {
			db.data[i] = connection
			if err := db.save(); err != nil {
				return err
			}
			return nil
		}
	}

	return nil
}

func (db *JsonLinesDatabase) Remove(id string) error {
	if err := db.ensureLoaded(); err != nil {
		return err
	}

	newData := make([]*Connection, 0)
	for _, item := range db.data {
		if item.Id != id {
			newData = append(newData, item)
		}
	}

	db.data = newData
	if err := db.save(); err != nil {
		return err
	}

	return nil
}
