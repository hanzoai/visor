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

package routers

import (
	"context"
	"sync"
	"time"

	beegoContext "github.com/beego/beego/context"
	"github.com/casvisor/casvisor/object"
)

var (
	lastAPITime             time.Time
	isUpdateInProgress      bool
	isUpdateInProgressMutex sync.Mutex
	interval                = 30 * time.Second
)

func MetricFilter(ctx *beegoContext.Context) {
	lastAPITime = time.Now()

	isUpdateInProgressMutex.Lock()
	defer isUpdateInProgressMutex.Unlock()
	if !isUpdateInProgress {
		isUpdateInProgress = true
		go updateAssetMetrics()
	}
}

func updateAssetMetrics() {
	defer func() {
		isUpdateInProgressMutex.Lock()
		isUpdateInProgress = false
		isUpdateInProgressMutex.Unlock()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case <-ticker.C:
				if time.Since(lastAPITime) > interval {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			object.CloseSshClients()
			return
		default:
			object.RunUpdateAssetMetrics()
		}
	}
}
