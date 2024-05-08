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

package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/casvisor/casvisor/util"
	"github.com/pkg/sftp"
)

type SftpProvider struct {
	sftpClient *sftp.Client
	sessionId  string
}

func NewSftpProvider(sessionId string) (*SftpProvider, error) {
	sftpClient, err := GetSftpClient(sessionId)
	if err != nil {
		return nil, err
	}

	return &SftpProvider{sftpClient: sftpClient, sessionId: sessionId}, nil
}

func GetSftpClient(sessionId string) (*sftp.Client, error) {
	gSession := util.GlobalSessionManager.Get(sessionId)
	if gSession == nil {
		return nil, errors.New("session not found")
	}

	if gSession.Terminal.SftpClient == nil {
		sftpClient, err := sftp.NewClient(gSession.Terminal.SshClient)
		if err != nil {
			return nil, err
		}
		gSession.Terminal.SftpClient = sftpClient
	}

	return gSession.Terminal.SftpClient, nil
}

func (p *SftpProvider) ListObjects(dir string) ([]*Object, error) {
	fileInfos, err := p.sftpClient.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	files := make([]*Object, 0)
	for _, fileInfo := range fileInfos {
		Object := &Object{
			Name:         fileInfo.Name(),
			Key:          path.Join(dir, fileInfo.Name()),
			LastModified: fileInfo.ModTime().String(),
			Size:         fileInfo.Size(),
			IsDir:        fileInfo.IsDir(),
			Mode:         fileInfo.Mode().String(),
			Url:          fmt.Sprintf("/api/get-file?id=%s&key=%s", p.sessionId, path.Join(dir, fileInfo.Name())),
		}
		files = append(files, Object)
	}

	return files, nil
}

func (p *SftpProvider) PutObject(user string, parent string, key string, fileBuffer *bytes.Buffer) (string, error) {
	fullPath := path.Join(parent, key)
	err := p.sftpClient.MkdirAll(path.Dir(fullPath))
	if err != nil {
		return "", err
	}

	dstFile, err := p.sftpClient.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, fileBuffer)
	if err != nil {
		return "", err
	}

	return fullPath, nil
}

func (p *SftpProvider) DeleteObject(key string) error {
	stat, err := p.sftpClient.Stat(key)
	if err != nil {
		return err
	}

	if stat.IsDir() {
		fileInfos, err := p.sftpClient.ReadDir(key)
		if err != nil {
			return err
		}

		for i := range fileInfos {
			if err := p.sftpClient.Remove(path.Join(key, fileInfos[i].Name())); err != nil {
				return err
			}
		}

		if err := p.sftpClient.RemoveDirectory(key); err != nil {
			return err
		}
	} else {
		if err := p.sftpClient.Remove(key); err != nil {
			return err
		}
	}
	return nil
}

func (p *SftpProvider) Mkdir(key string) error {
	err := p.sftpClient.Mkdir(key)
	if err != nil {
		return err
	}
	return nil
}
