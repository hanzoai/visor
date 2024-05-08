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

package controllers

import (
	"bytes"
	"fmt"
	"io"
	"path"

	"github.com/casvisor/casvisor/object"
	"github.com/casvisor/casvisor/storage"
	"github.com/casvisor/casvisor/util"
)

// UpdateFile
// @Title UpdateFile
// @Tag File API
// @Description update file
// @Param sessionId query string true "The id of the session"
// @Param filename query string true "The name of the file"
// @Success 200 {object} controllers.Response The Response object
// @router /update-file [post]
func (c *ApiController) UpdateFile() {
	c.ResponseOk()
}

// AddFile
// @Title AddFile
// @Tag File API
// @Description add file
// @Param key query string true "The directory of the file"
// @Param sessionId query string true "The id of the session"
// @Success 200 {object} controllers.Response The Response object
// @router /add-file [post]
func (c *ApiController) AddFile() {
	userName := c.GetSessionUsername()
	sessionId := c.Input().Get("id")
	key := c.Input().Get("key")
	isLeaf := c.Input().Get("isLeaf") == "1"

	session, err := object.GetConnSession(sessionId)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if session == nil {
		c.ResponseError("session not found")
		return
	}

	provider, err := storage.GetStorageProvider(session.Protocol, sessionId, "")
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	if isLeaf {
		_, file, err := c.GetFile("file")
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		srcFile, err := file.Open()
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		var fileBuffer *bytes.Buffer
		fileBuffer = bytes.NewBuffer(nil)
		_, err = io.Copy(fileBuffer, srcFile)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		_, err = provider.PutObject(userName, key, file.Filename, fileBuffer)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		addRecordForFile(c, userName, "Upload", sessionId, key, file.Filename, false)
		c.ResponseOk()
	} else {
		err = provider.Mkdir(key)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		addRecordForFile(c, userName, "MkDir", sessionId, key, "", false)
		c.ResponseOk()
	}
}

// DownloadFile
// @Title DownloadFile
// @Tag File API
// @Description download file
// @Param sessionId query string true "The id of the session"
// @Param key query string true "The direction of the file"
// @Success 200 {object} controllers.Response The Response object
// @router /get-file [get]
func (c *ApiController) DownloadFile() {
	userName := c.GetSessionUsername()
	sessionId := c.Input().Get("id")
	key := c.Input().Get("key")

	session, err := object.GetConnSession(sessionId)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if session == nil {
		c.ResponseError("session not found")
		return
	}

	sftpClient, err := storage.GetSftpClient(sessionId)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	dstFile, err := sftpClient.Open(key)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	defer dstFile.Close()

	addRecordForFile(c, userName, "Download", sessionId, key, "", false)
	filenameWithSuffix := path.Base(key)
	c.Ctx.ResponseWriter.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filenameWithSuffix))
	c.Ctx.ResponseWriter.Header().Set("Content-Type", "application/octet-stream")
	if _, err := dstFile.WriteTo(c.Ctx.ResponseWriter); err != nil {
		c.ResponseError(err.Error())
		return
	}
}

// DeleteFile
// @Title DeleteFile
// @Tag File API
// @Description delete file
// @Param sessionId query string true "The id of the session"
// @Param key query string true "The direction of the file"
// @Success 200 {object} controllers.Response The Response object
// @router /delete-file [post]
func (c *ApiController) DeleteFile() {
	userName := c.GetSessionUsername()
	sessionId := c.Input().Get("id")
	key := c.Input().Get("key")

	session, err := object.GetConnSession(sessionId)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if session == nil {
		c.ResponseError("session not found")
		return
	}

	provider, err := storage.GetStorageProvider(session.Protocol, sessionId, "")
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	err = provider.DeleteObject(key)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	addRecordForFile(c, userName, "Delete", sessionId, key, "", false)
	c.ResponseOk()
}

// GetFiles
// @Title GetFiles
// @Tag File API
// @Description list files
// @Param sessionId query string true "The id of the session"
// @Param key query string true "The direction of the file"
// @Success 200 {object} controllers.Response The Response object
// @router /get-files [get]
func (c *ApiController) GetFiles() {
	userName := c.GetSessionUsername()
	sessionId := c.Input().Get("id")
	key := c.Input().Get("key")
	mode := c.Input().Get("mode")

	session, err := object.GetConnSession(sessionId)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if session == nil {
		c.ResponseError("session not found")
		return
	}

	provider, err := storage.GetStorageProvider(session.Protocol, sessionId, "")
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	objects, err := provider.ListObjects(key)

	if mode == "store" {
		store := object.Store{
			Owner:       session.Owner,
			Name:        session.Name,
			DisplayName: session.Asset,
			CreatedTime: util.GetCurrentTime(),
		}

		host := c.Ctx.Request.Host
		origin := getOriginFromHost(host)
		err := store.Populate(origin, key, objects)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		addRecordForFile(c, userName, "Ls", sessionId, key, "", false)
		c.ResponseOk(store)
	} else {
		files := []*object.File{}
		for _, o := range objects {
			file := object.File{
				Key:         o.Key,
				Title:       o.Name,
				Size:        o.Size,
				CreatedTime: o.LastModified,
				IsLeaf:      !o.IsDir,
				Url:         o.Url,
			}
			files = append(files, &file)
		}

		addRecordForFile(c, userName, "Ls", sessionId, key, "", false)
		c.ResponseOk(files)
	}
}
