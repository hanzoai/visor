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
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/beego/beego/context"
	"github.com/casvisor/casvisor/conf"
	"github.com/casvisor/casvisor/util"
)

const (
	headerAllowOrigin  = "Access-Control-Allow-Origin"
	headerAllowMethods = "Access-Control-Allow-Methods"
	headerAllowHeaders = "Access-Control-Allow-Headers"
)

var (
	oldCasdoorEndpoint     = "https://door.casdoor.com"
	newCasdoorEndpoint     = conf.GetConfigString("casdoorEndpoint")
	oldClientId            = "b108dacba027db36ec26"
	newClientId            = conf.GetConfigString("clientId")
	oldCasdoorOrganization = "casbin"
	newCasdoorOrganization = conf.GetConfigString("casdoorOrganization")
	oldCasdoorApplication  = "app-casvisor"
	newCasdoorApplication  = conf.GetConfigString("casdoorApplication")
)

func TransparentStatic(ctx *context.Context) {
	urlPath := ctx.Request.URL.Path
	if strings.HasPrefix(urlPath, "/api/") || strings.HasPrefix(urlPath, "/dbgate") {
		return
	}

	if strings.HasPrefix(urlPath, "/storage") {
		ctx.Output.Header(headerAllowOrigin, "*")
		ctx.Output.Header(headerAllowMethods, "POST, GET, OPTIONS, DELETE")
		ctx.Output.Header(headerAllowHeaders, "Content-Type, Authorization")

		urlPath = strings.TrimPrefix(urlPath, "/storage/")
		urlPath = strings.Replace(urlPath, "|", ":", 1)
		makeGzipResponse(ctx.ResponseWriter, ctx.Request, urlPath)
		return
	}

	path := "web/build"
	if urlPath == "/" {
		path += "/index.html"
	} else {
		path += urlPath
	}

	if !util.FileExist(path) {
		path = "web/build/index.html"
	}
	makeGzipResponse(ctx.ResponseWriter, ctx.Request, path)
}

func serveFileWithReplace(w http.ResponseWriter, r *http.Request, name string) {
	f, err := os.Open(filepath.Clean(name))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	d, err := f.Stat()
	if err != nil {
		panic(err)
	}

	content := util.ReadStringFromPath(name)
	if oldCasdoorEndpoint != newCasdoorEndpoint {
		content = strings.ReplaceAll(content, fmt.Sprintf("\"%s\"", oldCasdoorEndpoint), fmt.Sprintf("\"%s\"", newCasdoorEndpoint))
	}
	if oldClientId != newClientId {
		content = strings.ReplaceAll(content, fmt.Sprintf("\"%s\"", oldClientId), fmt.Sprintf("\"%s\"", newClientId))
	}
	if oldCasdoorOrganization != newCasdoorOrganization {
		content = strings.ReplaceAll(content, fmt.Sprintf("\"%s\"", oldCasdoorOrganization), fmt.Sprintf("\"%s\"", newCasdoorOrganization))
	}
	if oldCasdoorApplication != newCasdoorApplication {
		content = strings.ReplaceAll(content, fmt.Sprintf("\"%s\"", oldCasdoorApplication), fmt.Sprintf("\"%s\"", newCasdoorApplication))
	}

	http.ServeContent(w, r, d.Name(), d.ModTime(), strings.NewReader(content))
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func makeGzipResponse(w http.ResponseWriter, r *http.Request, path string) {
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		serveFileWithReplace(w, r, path)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	gzw := gzipResponseWriter{Writer: gz, ResponseWriter: w}
	serveFileWithReplace(gzw, r, path)
}
