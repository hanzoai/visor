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

package routers

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/beego/beego/context"
	"github.com/casvisor/casvisor/conf"
)

func ProxyFilter(ctx *context.Context) {
	user := GetSessionUser(ctx)
	if user == nil {
		requestDeny(ctx)
		return
	}

	requestPath := ctx.Request.RequestURI
	dbgateEndpoint := conf.GetConfigString("dbgateEndpoint")

	targetURL, err := url.Parse(dbgateEndpoint + requestPath)
	if err != nil {
		responseError(ctx, fmt.Sprintf("Invalid target URL: %s", err))
		return
	}

	originalQuery := ctx.Request.URL.RawQuery
	targetURLWithQuery := targetURL
	if originalQuery != "" {
		parsedQuery, err := url.ParseQuery(originalQuery)
		if err != nil {
			responseError(ctx, fmt.Sprintf("Invalid query string: %s", err))
			return
		}

		targetQuery := targetURL.Query()
		for key, values := range parsedQuery {
			for _, value := range values {
				targetQuery.Add(key, value)
			}
		}
		targetURLWithQuery.RawQuery = targetQuery.Encode()
	}

	target, err := url.Parse(targetURLWithQuery.String())
	if err != nil {
		responseError(ctx, fmt.Sprintf("Invalid target URL: %s", err))
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(r *http.Request) {
		r.URL = target

		if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" && xff != clientIP {
				newXff := fmt.Sprintf("%s, %s", xff, clientIP)
				r.Header.Set("X-Real-Ip", newXff)
			} else {
				r.Header.Set("X-Real-Ip", clientIP)
			}
		}

		fileExt := filepath.Ext(r.URL.Path)
		contentType := mime.TypeByExtension(fileExt)
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
	}

	// ErrorHandler is called when the proxy encounters an error.
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		print(err.Error())
		if strings.Contains(err.Error(), "No connection could be made because the target machine actively refused it") {
			responseError(ctx, "dbgate service is not available")
			return
		}

		responseError(ctx, err.Error())
	}

	proxy.ServeHTTP(ctx.ResponseWriter, ctx.Request)
}
