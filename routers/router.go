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

package routers

import (
	"github.com/beego/beego"
	"github.com/casvisor/casvisor/controllers"
)

func init() {
	initAPI()
}

func initAPI() {
	ns := beego.NewNamespace("/",
		beego.NSNamespace("/api",
			beego.NSInclude(
				&controllers.ApiController{},
			),
		),
		beego.NSNamespace("",
			beego.NSInclude(
				&controllers.RootController{},
			),
		),
	)
	beego.AddNamespace(ns)

	beego.Router("/api/signin", &controllers.ApiController{}, "POST:Signin")
	beego.Router("/api/signout", &controllers.ApiController{}, "POST:Signout")
	beego.Router("/api/get-account", &controllers.ApiController{}, "GET:GetAccount")

	beego.Router("/api/get-records", &controllers.ApiController{}, "GET:GetRecords")
	beego.Router("/api/get-record", &controllers.ApiController{}, "GET:GetRecord")
	beego.Router("/api/update-record", &controllers.ApiController{}, "POST:UpdateRecord")
	beego.Router("/api/add-record", &controllers.ApiController{}, "POST:AddRecord")
	beego.Router("/api/delete-record", &controllers.ApiController{}, "POST:DeleteRecord")

	beego.Router("/api/get-assets", &controllers.ApiController{}, "GET:GetAssets")
	beego.Router("/api/get-asset", &controllers.ApiController{}, "GET:GetAsset")
	beego.Router("/api/update-asset", &controllers.ApiController{}, "POST:UpdateAsset")
	beego.Router("/api/add-asset", &controllers.ApiController{}, "POST:AddAsset")
	beego.Router("/api/delete-asset", &controllers.ApiController{}, "POST:DeleteAsset")
	beego.Router("/api/refresh-asset-status", &controllers.ApiController{}, "POST:RefreshAssetStatus")
	beego.Router("/api/detect-assets", &controllers.ApiController{}, "POST:DetectAssets")
	beego.Router("/api/get-detected-assets", &controllers.ApiController{}, "GET:GetDetectedAssets")
	beego.Router("/api/add-detected-asset", &controllers.ApiController{}, "POST:AddDetectedAsset")
	beego.Router("/api/delete-detected-assets", &controllers.ApiController{}, "POST:DeleteDetectedAssets")

	beego.Router("/api/get-sessions", &controllers.ApiController{}, "GET:GetSessions")
	beego.Router("/api/get-session", &controllers.ApiController{}, "GET:GetConnSession")
	beego.Router("/api/update-session", &controllers.ApiController{}, "POST:UpdateSession")
	beego.Router("/api/add-session", &controllers.ApiController{}, "POST:AddSession")
	beego.Router("/api/delete-session", &controllers.ApiController{}, "POST:DeleteSession")
	beego.Router("/api/start-session", &controllers.ApiController{}, "POST:StartSession")
	beego.Router("/api/stop-session", &controllers.ApiController{}, "POST:StopSession")

	beego.Router("/api/add-asset-tunnel", &controllers.ApiController{}, "POST:AddAssetTunnel")
	beego.Router("/api/get-asset-tunnel", &controllers.ApiController{}, "GET:GetAssetTunnel")

	beego.Router("/api/get-commands", &controllers.ApiController{}, "GET:GetCommands")
	beego.Router("/api/get-command", &controllers.ApiController{}, "GET:GetCommand")
	beego.Router("/api/update-command", &controllers.ApiController{}, "POST:UpdateCommand")
	beego.Router("/api/add-command", &controllers.ApiController{}, "POST:AddCommand")
	beego.Router("/api/delete-command", &controllers.ApiController{}, "POST:DeleteCommand")
	beego.Router("/api/exec-command", &controllers.ApiController{}, "GET:ExecCommand")
	beego.Router("/api/get-exec-output", &controllers.ApiController{}, "GET:GetExecOutput")

	beego.Router("/api/update-file", &controllers.ApiController{}, "POST:UpdateFile")
	beego.Router("/api/delete-file", &controllers.ApiController{}, "POST:DeleteFile")
	beego.Router("/api/get-file", &controllers.ApiController{}, "GET:DownloadFile")
	beego.Router("/api/add-file", &controllers.ApiController{}, "POST:AddFile")
	beego.Router("/api/get-files", &controllers.ApiController{}, "POST:GetFiles")

	beego.Router("/api/get-permissions", &controllers.ApiController{}, "GET:GetPermissions")
	beego.Router("/api/get-permission", &controllers.ApiController{}, "GET:GetPermission")
	beego.Router("/api/update-permission", &controllers.ApiController{}, "POST:UpdatePermission")
	beego.Router("/api/add-permission", &controllers.ApiController{}, "POST:AddPermission")
	beego.Router("/api/delete-permission", &controllers.ApiController{}, "POST:DeletePermission")

	beego.Router("/agent/get-system-info", &controllers.RootController{}, "GET:GetSystemInfo")
}
