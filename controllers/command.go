// Copyright 2024 The Casbin Authors. All Rights Reserved.
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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/beego/beego/utils/pagination"
	"github.com/casvisor/casvisor/conf"
	"github.com/casvisor/casvisor/object"
	"github.com/casvisor/casvisor/util"
	"github.com/yahoo/vssh"
)

// GetCommands
// @Title GetCommands
// @Tag Command API
// @Description get all commands
// @Param   pageSize     query    string  true        "The size of each page"
// @Param   p     query    string  true        "The number of the page"
// @Success 200 {object} object.Command The Response object
// @router /get-commands [get]
func (c *ApiController) GetCommands() {
	owner := c.Input().Get("owner")
	limit := c.Input().Get("pageSize")
	page := c.Input().Get("p")
	field := c.Input().Get("field")
	value := c.Input().Get("value")
	sortField := c.Input().Get("sortField")
	sortOrder := c.Input().Get("sortOrder")

	if limit == "" || page == "" {
		commands, err := object.GetCommands(owner)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		c.ResponseOk(commands)
	} else {
		limit := util.ParseInt(limit)

		count, err := object.GetCommandCount(owner, field, value)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		paginator := pagination.SetPaginator(c.Ctx, limit, count)
		commands, err := object.GetPaginationCommands(owner, paginator.Offset(), limit, field, value, sortField, sortOrder)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		c.ResponseOk(commands, paginator.Nums())
	}
}

// GetCommand
// @Title GetCommand
// @Tag Command API
// @Description get the command
// @Param   id	 query    string  true        "The id ( owner/name ) of the command"
// @Success 200 {object} object.Command The Response object
// @router /get-command [get]
func (c *ApiController) GetCommand() {
	id := c.Input().Get("id")
	command, err := object.GetCommand(id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(command)
}

// UpdateCommand
// @Title UpdateCommand
// @Tag Command API
// @Description update the command
// @Param   id	 query    string  true        "The id ( owner/name ) of the command"
// @Param   command	 body    object.Command  true        "The command object"
// @Success 200 {string} The Response object
// @router /update-command [post]
func (c *ApiController) UpdateCommand() {
	id := c.Input().Get("id")

	var command object.Command
	err := json.Unmarshal(c.Ctx.Input.RequestBody, &command)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.Data["json"] = wrapActionResponse(object.UpdateCommand(id, &command))
	c.ServeJSON()
}

// DeleteCommand
// @Title DeleteCommand
// @Tag Command API
// @Description delete the command
// @Param   id	 query    string  true        "The id ( owner/name ) of the command"
// @Success 200 {string} The Response object
// @router /delete-command [post]
func (c *ApiController) DeleteCommand() {
	var command object.Command
	err := json.Unmarshal(c.Ctx.Input.RequestBody, &command)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.Data["json"] = wrapActionResponse(object.DeleteCommand(&command))
	c.ServeJSON()
}

// AddCommand
// @Title AddCommand
// @Tag Command API
// @Description add a command
// @Param   command	 body    object.Command  true        "The command object"
// @Success 200 {string} The Response object
// @router /add-command [post]
func (c *ApiController) AddCommand() {
	var command object.Command
	err := json.Unmarshal(c.Ctx.Input.RequestBody, &command)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.Data["json"] = wrapActionResponse(object.AddCommand(&command))
	c.ServeJSON()
}

// ExecCommand
// @Title ExecCommand
// @Tag Command API
// @Description execute the command
// @Param   id	 query    string  true        "The id ( owner/name ) of the command"
// @Param   assetId	     query    string  true        "The id of the asset"
// @Success 200 {stream} string "An event stream of the command output in multiple ssh terminal"
// @router /exec-command [get]
func (c *ApiController) ExecCommand() {
	c.Ctx.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	c.Ctx.ResponseWriter.Header().Set("Cache-Control", "no-cache")
	c.Ctx.ResponseWriter.Header().Set("Connection", "keep-alive")

	commandId := c.Input().Get("id")
	assetId := c.Input().Get("assetId")

	command, err := object.GetCommand(commandId)
	if err != nil {
		c.ResponseErrorStream(err.Error())
		return
	}
	if command == nil {
		c.ResponseErrorStream("Command not found")
		return
	}

	asset, err := object.GetAsset(assetId)
	if err != nil {
		c.ResponseErrorStream(err.Error())
		return
	}
	if asset == nil {
		c.ResponseErrorStream("Asset not found")
		return
	}

	vs := vssh.New().Start()
	config := vssh.GetConfigUserPass(asset.Username, asset.Password)
	var addr string
	if asset.GatewayPort != 0 {
		addr = fmt.Sprintf("%s:%d", conf.GatewayAddr.IP, asset.GatewayPort)
	} else {
		addr = fmt.Sprintf("%s:%d", asset.Endpoint, asset.Port)
	}
	err = vs.AddClient(addr, config, vssh.SetMaxSessions(4))
	if err != nil {
		c.ResponseErrorStream(err.Error())
		return
	}
	_, err = vs.Wait()
	if err != nil {
		c.ResponseErrorStream(err.Error())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := command.Command
	timeout, _ := time.ParseDuration("10s")
	respChan := vs.Run(ctx, cmd, timeout)

	resp := <-respChan
	if err := resp.Err(); err != nil {
		c.ResponseErrorStream(err.Error())
		return
	}

	stream := resp.GetStream()
	defer stream.Close()

	writer := &RefinedWriter{*c.Ctx.ResponseWriter, *NewCleaner(6), []byte{}}

	for stream.ScanStdout() {
		txt := stream.TextStdout()
		if _, err = fmt.Fprintf(writer, "event: message\ndata: %s\n\n", txt); err != nil {
			c.ResponseErrorStream(err.Error())
			return
		}
		writer.Flush()
	}

	if writer.writerCleaner.cleaned == false {
		cleanedData := writer.writerCleaner.GetCleanedData()
		writer.buf = append(writer.buf, []byte(cleanedData)...)
		jsonData, err := ConvertMessageDataToJSON(cleanedData)
		if err != nil {
			c.ResponseErrorStream(err.Error())
			return
		}

		_, err = writer.ResponseWriter.Write([]byte(fmt.Sprintf("event: message\ndata: %s\n\n", jsonData)))
		if err != nil {
			c.ResponseErrorStream(err.Error())
			return
		}

		writer.Flush()
	}

	event := fmt.Sprintf("event: end\ndata: %s\n\n", "end")
	_, err = c.Ctx.ResponseWriter.Write([]byte(event))
	if err != nil {
		c.ResponseErrorStream(err.Error())
		return
	}
}

// GetExecOutput
// @Title GetExecOutput
// @Tag Command API
// @Description get the output of the command
// @Param   id	 query    string  true        "The id ( owner/name ) of the command"
// @Success 200 {string} The Response object
// @router /get-exec-output [get]

func (c *ApiController) GetExecOutput() {
	id := c.Input().Get("id")

	command, err := object.GetCommand(id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if command == nil {
		c.ResponseError("Command not found")
		return
	}

	output, err := object.GetExecOutput(command)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(string(outputJSON))
}
