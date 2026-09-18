// Copyright 2026 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package main

import (
	"errors"
)

// git passes argv through to git, run in the workspace. The ceiling keeps a
// repository search from walking above the workspace root, so a request that
// names no repository fails in the workspace instead of finding one outside
// it.
func (d *daemon) git(r *req) *rep {
	if r.command != "" {
		return fail(r, errors.New("git takes argv, not a command"))
	}
	if len(r.argv) == 0 {
		return fail(r, errors.New("git needs argv"))
	}
	argv := append([]string{"git"}, r.argv...)
	return d.run(r, argv, []string{
		"GIT_CEILING_DIRECTORIES=" + d.root.name,
		"GIT_DISCOVERY_ACROSS_FILESYSTEM=0",
	})
}
