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

package object

import (
	"fmt"
	"strings"

	"github.com/casvisor/casvisor/storage"
)

type File struct {
	Key          string  `xorm:"varchar(100)" json:"key"`
	Title        string  `xorm:"varchar(100)" json:"title"`
	Size         int64   `json:"size"`
	LastModified string  `xorm:"varchar(100)" json:"lastModified"`
	Mode         string  `xorm:"varchar(100)" json:"mode"`
	CreatedTime  string  `xorm:"varchar(100)" json:"createdTime"`
	IsLeaf       bool    `json:"isLeaf"`
	Url          string  `xorm:"varchar(255)" json:"url"`
	Children     []*File `xorm:"varchar(1000)" json:"children"`

	ChildrenMap map[string]*File `xorm:"-" json:"-"`
}

type Properties struct {
	CollectedTime string `xorm:"varchar(100)" json:"collectedTime"`
	Subject       string `xorm:"varchar(100)" json:"subject"`
}

type Store struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`
	DisplayName string `xorm:"varchar(100)" json:"displayName"`

	StorageProvider   string `xorm:"varchar(100)" json:"storageProvider"`
	ImageProvider     string `xorm:"varchar(100)" json:"imageProvider"`
	SplitProvider     string `xorm:"varchar(100)" json:"splitProvider"`
	ModelProvider     string `xorm:"varchar(100)" json:"modelProvider"`
	EmbeddingProvider string `xorm:"varchar(100)" json:"embeddingProvider"`

	MemoryLimit  int    `json:"memoryLimit"`
	Frequency    int    `json:"frequency"`
	LimitMinutes int    `json:"limitMinutes"`
	Welcome      string `xorm:"varchar(100)" json:"welcome"`
	Prompt       string `xorm:"mediumtext" json:"prompt"`

	FileTree      *File                  `xorm:"mediumtext" json:"fileTree"`
	PropertiesMap map[string]*Properties `xorm:"mediumtext" json:"propertiesMap"`
}

func (store *Store) createPathIfNotExisted(tokens []string, size int64, url string, lastModifiedTime string, isLeaf bool) {
	currentFile := store.FileTree
	for i, token := range tokens {
		if currentFile.Children == nil {
			currentFile.Children = []*File{}
		}
		if currentFile.ChildrenMap == nil {
			currentFile.ChildrenMap = map[string]*File{}
		}

		tmpFile, ok := currentFile.ChildrenMap[token]
		if ok {
			currentFile = tmpFile
			continue
		}

		key := strings.Join(tokens[:i+1], "/")
		newFile := &File{
			Key:         "/" + key,
			Title:       token,
			IsLeaf:      isLeaf,
			Url:         url,
			ChildrenMap: map[string]*File{},
		}

		if i == len(tokens)-1 {
			newFile.Size = size
			newFile.CreatedTime = lastModifiedTime

			if token == "_hidden.ini" {
				continue
			}
		} else if i == len(tokens)-2 {
			if tokens[len(tokens)-1] == "_hidden.ini" {
				newFile.CreatedTime = lastModifiedTime
			}
		}

		currentFile.Children = append(currentFile.Children, newFile)
		currentFile.ChildrenMap[token] = newFile
		currentFile = newFile
	}
}

func isObjectLeaf(object *File) bool {
	isLeaf := true
	if object.Key[len(object.Key)-1] == '/' {
		isLeaf = false
	}
	return isLeaf
}

func (store *Store) Populate(origin string, key string, objects []*storage.Object) error {
	if store.FileTree == nil {
		store.FileTree = &File{
			Key:         key,
			Title:       store.DisplayName,
			CreatedTime: store.CreatedTime,
			IsLeaf:      false,
			Url:         "",
			ChildrenMap: map[string]*File{},
		}
	}

	sortedObjects := []*storage.Object{}
	for _, object := range objects {
		if strings.HasSuffix(object.Key, "/_hidden.ini") {
			sortedObjects = append(sortedObjects, object)
		}
	}
	for _, object := range objects {
		if !strings.HasSuffix(object.Key, "/_hidden.ini") {
			sortedObjects = append(sortedObjects, object)
		}
	}

	for _, object := range sortedObjects {
		lastModifiedTime := object.LastModified
		size := object.Size

		var url string
		url, err := getUrlFromPath(object.Url, origin)
		if err != nil {
			return err
		}

		tokens := strings.Split(strings.Trim(object.Key, "/"), "/")
		store.createPathIfNotExisted(tokens, size, url, lastModifiedTime, !object.IsDir)
	}

	return nil
}

func (store *Store) GetStorageProviderObj() (storage.StorageProvider, error) {
	session, err := GetConnSession(store.StorageProvider)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("session not found")
	}

	provider, err := storage.GetStorageProvider(session.Protocol, store.StorageProvider, "")
	return provider, nil
}
