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

package metric

import "sync"

type Runner struct {
	wg     sync.WaitGroup
	errors []error
	mux    sync.Mutex
}

func (r *Runner) Add(f func() error) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := f(); err != nil {
			r.addError(err)
		}
	}()
}

func (r *Runner) addError(err error) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.errors = append(r.errors, err)
}

func (r *Runner) Wait() []error {
	r.wg.Wait()
	return r.errors
}
