// Copyright 2023 The casbin Authors. All Rights Reserved.
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
	"fmt"
	"strconv"

	"github.com/casvisor/casvisor/conf"
	"github.com/casvisor/casvisor/dbgate"
	"github.com/casvisor/casvisor/util"
	"xorm.io/core"
)

var (
	dataStore         *dbgate.JsonLinesDatabase
	RestartClientChan chan string
)

const (
	AssetStatusRunning = "Running"
	AssetStatusStopped = "Stopped"
)

func init() {
	dataStore = dbgate.NewConnectionDataStore()
	RestartClientChan = make(chan string)
}

type Service struct {
	No             int    `json:"no"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Port           int    `json:"port"`
	ProcessId      int    `json:"processId"`
	ExpectedStatus string `json:"expectedStatus"`
	Status         string `json:"status"`
	SubStatus      string `json:"subStatus"`
	Message        string `json:"message"`
}

type RemoteApp struct {
	No            int    `json:"no"`
	RemoteAppName string `xorm:"varchar(100)" json:"remoteAppName"`
	RemoteAppDir  string `xorm:"varchar(100)" json:"remoteAppDir"`
	RemoteAppArgs string `xorm:"varchar(100)" json:"remoteAppArgs"`
}

type Asset struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`
	DisplayName string `xorm:"varchar(100)" json:"displayName"`

	Category string `xorm:"varchar(20)" json:"category"`
	Type     string `xorm:"varchar(100)" json:"type"`
	Tag      string `xorm:"varchar(200)" json:"tag"`

	Endpoint string `xorm:"varchar(100)" json:"endpoint"`
	Port     int    `json:"port"`
	Username string `xorm:"varchar(100)" json:"username"`
	Password string `xorm:"varchar(200)" json:"password"`

	Os              string       `xorm:"varchar(100)" json:"os"`
	Language        string       `xorm:"varchar(20)" json:"language"`
	AutoQuery       bool         `json:"autoQuery"`
	IsPermanent     bool         `json:"isPermanent"`
	EnableRemoteApp bool         `json:"enableRemoteApp"`
	RemoteApps      []*RemoteApp `json:"remoteApps"`
	Services        []*Service   `json:"services"`
	GatewayPort     int          `json:"gatewayPort"`
	EnableSsh       bool         `json:"enableSsh"`
	SshPort         int          `json:"sshPort"`
	Status          string       `xorm:"varchar(20)" json:"status"`
	DiskCurrent     int64        `json:"diskCurrent"`
	DiskTotal       int64        `json:"diskTotal"`
	MemCurrent      int64        `json:"memCurrent"`
	MemTotal        int64        `json:"memTotal"`
	CpuCurrent      float32      `json:"cpuCurrent"`
	CpuTotal        int          `json:"cpuTotal"`
	Id              string       `xorm:"varchar(100)" json:"id"`
	DatabaseUrl     string       `xorm:"varchar(200)" json:"databaseUrl"`
	UseDatabaseUrl  bool         `json:"useDatabaseUrl"`
	DatabaseFile    string       `xorm:"varchar(200)" json:"databaseFile"`
	SocketPath      string       `xorm:"varchar(200)" json:"socketPath"`
	AuthType        string       `xorm:"varchar(100)" json:"authType"`
	DefaultDatabase string       `xorm:"varchar(100)" json:"defaultDatabase"`
	IsReadOnly      bool         `json:"isReadOnly"`
}

func GetAssetCount(owner, field, value string) (int64, error) {
	session := GetSession(owner, -1, -1, field, value, "", "")
	return session.Count(&Asset{})
}

func GetAssets(owner string) ([]*Asset, error) {
	assets := []*Asset{}
	err := adapter.Engine.Desc("created_time").Find(&assets, &Asset{Owner: owner})
	if err != nil {
		return assets, err
	}

	return assets, nil
}

func GetPaginationAssets(owner string, offset, limit int, field, value, sortField, sortOrder string) ([]*Asset, error) {
	assets := []*Asset{}
	session := GetSession(owner, offset, limit, field, value, sortField, sortOrder)
	err := session.Find(&assets)
	if err != nil {
		return assets, err
	}

	return assets, nil
}

func getAsset(owner string, name string) (*Asset, error) {
	if owner == "" || name == "" {
		return nil, nil
	}

	asset := Asset{Owner: owner, Name: name}
	existed, err := adapter.Engine.Get(&asset)
	if err != nil {
		return &asset, err
	}

	if existed {
		return &asset, nil
	} else {
		return nil, nil
	}
}

func GetAsset(id string) (*Asset, error) {
	owner, name := util.GetOwnerAndNameFromId(id)
	return getAsset(owner, name)
}

func GetMaskedAsset(asset *Asset, errs ...error) (*Asset, error) {
	if len(errs) > 0 && errs[0] != nil {
		return nil, errs[0]
	}

	if asset == nil {
		return nil, nil
	}

	if asset.Password != "" {
		asset.Password = "***"
	}
	return asset, nil
}

func GetMaskedAssets(assets []*Asset, errs ...error) ([]*Asset, error) {
	if len(errs) > 0 && errs[0] != nil {
		return nil, errs[0]
	}

	var err error
	for _, asset := range assets {
		asset, err = GetMaskedAsset(asset)
		if err != nil {
			return nil, err
		}
	}

	return assets, nil
}

func UpdateAsset(id string, asset *Asset) (bool, error) {
	owner, name := util.GetOwnerAndNameFromId(id)
	oldAsset, err := getAsset(owner, name)
	if err != nil {
		return false, err
	} else if oldAsset == nil {
		return false, nil
	}

	if name != asset.Name {
		err = assetChangeTrigger(name, asset.Name)
		if err != nil {
			return false, err
		}
	}

	if asset.Password == "***" {
		asset.Password = oldAsset.Password
	}

	affected, err := adapter.Engine.ID(core.PK{owner, name}).AllCols().Update(asset)
	if err != nil {
		return false, err
	}

	err = AssetHook(asset, oldAsset, "update")
	if err != nil {
		return false, err
	}

	return affected != 0, nil
}

func AddAsset(asset *Asset) (bool, error) {
	if asset.Id == "" {
		asset.Id = util.GenerateId()
	}
	affected, err := adapter.Engine.Insert(asset)
	if err != nil {
		return false, err
	}

	err = AssetHook(asset, nil, "insert")
	if err != nil {
		return false, err
	}

	return affected != 0, nil
}

func DeleteAsset(asset *Asset) (bool, error) {
	affected, err := adapter.Engine.ID(core.PK{asset.Owner, asset.Name}).Delete(&Asset{})
	if err != nil {
		return false, err
	}

	err = AssetHook(asset, nil, "delete")
	if err != nil {
		return false, err
	}

	return affected != 0, nil
}

func (asset *Asset) GetId() string {
	return fmt.Sprintf("%s/%s", asset.Owner, asset.Name)
}

func (asset *Asset) GetAddr() string {
	if asset == nil {
		return ""
	}

	var addr string
	if asset.GatewayPort != 0 {
		addr = fmt.Sprintf("%s:%d", conf.GatewayAddr.IP, asset.GatewayPort)
	} else if asset.EnableSsh {
		addr = fmt.Sprintf("%s:%d", asset.Endpoint, asset.SshPort)
	} else {
		addr = fmt.Sprintf("%s:%d", asset.Endpoint, asset.Port)
	}
	return addr
}

func GetAssetsByName(owner, name string, isAdmin bool) ([]*Asset, error) {
	if isAdmin {
		return GetAssets(owner)
	}

	assets := []*Asset{}
	// TODO get asset by call enforcer API

	return assets, nil
}

func (asset *Asset) toConnection() *dbgate.Connection {
	connection := &dbgate.Connection{
		Id:              asset.Id,
		Engine:          asset.Type,
		Server:          asset.Endpoint,
		User:            asset.Username,
		Password:        asset.Password,
		Port:            strconv.Itoa(asset.Port),
		DatabaseUrl:     asset.DatabaseUrl,
		UseDatabaseUrl:  asset.UseDatabaseUrl,
		DatabaseFile:    asset.DatabaseFile,
		SocketPath:      asset.SocketPath,
		AuthType:        asset.AuthType,
		DefaultDatabase: asset.DefaultDatabase,
		DisplayName:     asset.DisplayName,
		IsReadOnly:      asset.IsReadOnly,
	}
	return connection.TransferToSave()
}

func AssetHook(asset *Asset, oldAsset *Asset, action string) error {
	if action == "update" && asset.Category == "Machine" {
		if asset.GatewayPort != oldAsset.GatewayPort {
			RestartClientChan <- asset.Name
		}
	}

	if oldAsset != nil {
		if oldAsset.Category == "Database" && asset.Category != "Database" {
			err := dataStore.Remove(asset.Id)
			if err != nil {
				return err
			}
		}
		if oldAsset.Category != "Database" && asset.Category == "Database" {
			err := dataStore.Insert(asset.toConnection())
			if err != nil {
				return err
			}
		}
	}

	if asset.Category != "Database" {
		return nil
	}

	switch action {
	case "insert":
		err := dataStore.Insert(asset.toConnection())
		if err != nil {
			return err
		}
	case "update":
		err := dataStore.Update(asset.toConnection())
		if err != nil {
			return err
		}
	case "delete":
		err := dataStore.Remove(asset.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func assetChangeTrigger(oldName string, newName string) error {
	session := adapter.Engine.NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		return err
	}

	var commands []*Command
	err = adapter.Engine.Find(&commands)
	if err != nil {
		return err
	}
	for i, command := range commands {
		for j, asset := range command.Assets {
			if asset == oldName {
				commands[i].Assets[j] = newName
			}
		}
		_, err = session.ID(core.PK{command.Owner, command.Name}).Update(commands[i])
		if err != nil {
			return err
		}
	}

	return session.Commit()
}
