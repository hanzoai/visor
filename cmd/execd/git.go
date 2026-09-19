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
	"fmt"
	"path"
	"strings"
)

// git passes argv through to git, run in the workspace.
//
// dir is the one thing in a request that says where git works. Before the
// subcommand argv may carry -c and the options in inert, and nothing else:
// -C, --git-dir, --work-tree and --exec-path would move git or what it runs,
// and an option not on the list is refused too, since one that takes a value
// would hide the option after it. A GIT_* variable in env is refused, since
// GIT_DIR, GIT_WORK_TREE or GIT_CONFIG_GLOBAL there would name a place of the
// request's choosing. git reads the repository's configuration, the system's
// and what -c states; the global file is /dev/null, so a HOME in env picks
// none.
//
// The ceiling names the workspace's parent, not the workspace: git stops its
// repository search at a ceiling directory on the way up, and a ceiling equal
// to the directory it starts from is never on that way, so a ceiling of the
// workspace itself would let a search that starts at the workspace root find
// and write to a repository above it. With the parent as the ceiling, the
// search ends at the workspace — from the root and from any directory under
// it.
func (d *daemon) git(r *req) *rep {
	if r.command != "" {
		return fail(r, errors.New("git takes argv, not a command"))
	}
	if len(r.argv) == 0 {
		return fail(r, errors.New("git needs argv"))
	}
	if err := leading(r.argv); err != nil {
		return fail(r, err)
	}
	for _, v := range r.env {
		if name, _, _ := strings.Cut(v, "="); strings.HasPrefix(name, "GIT_") {
			return fail(r, fmt.Errorf("env %s: dir says where git works, and -c how", name))
		}
	}
	argv := append([]string{"git"}, r.argv...)
	return d.run(r, argv, []string{
		"GIT_CEILING_DIRECTORIES=" + path.Dir(d.root.name),
		"GIT_DISCOVERY_ACROSS_FILESYSTEM=0",
		"GIT_CONFIG_GLOBAL=/dev/null",
	})
}

// inert is what git may take before its subcommand besides -c: options that
// change how it reads and prints, and never where it works.
var inert = map[string]bool{
	"-P":                   true,
	"--no-pager":           true,
	"--no-optional-locks":  true,
	"--no-replace-objects": true,
	"--literal-pathspecs":  true,
	"--glob-pathspecs":     true,
	"--noglob-pathspecs":   true,
	"--icase-pathspecs":    true,
}

// leading checks argv up to the subcommand, the first word that is not an
// option. After it every word is the subcommand's own.
func leading(argv []string) error {
	for i := 0; i < len(argv); i++ {
		switch a := argv[i]; {
		case a == "-c":
			i++ // a setting, whatever it looks like
		case !strings.HasPrefix(a, "-"):
			return nil
		case !inert[a]:
			return fmt.Errorf("%s before the subcommand: dir says where git works, and -c how", a)
		}
	}
	return nil
}
