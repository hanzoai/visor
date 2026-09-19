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
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// tree lays out a workspace with something to escape to just outside it.
func tree(t *testing.T) (*root, string) {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{work, outside, filepath.Join(work, "src")} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(work, "src", "main.go"), "package main\n")
	write(t, filepath.Join(outside, "secret"), "not yours\n")
	if err := os.Symlink(outside, filepath.Join(work, "away")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("src", filepath.Join(work, "here")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(work, "src"), filepath.Join(work, "rooted")); err != nil {
		t.Fatal(err)
	}

	r, err := openRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.close() })
	return r, base
}

func write(t *testing.T, name, body string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRootOpensWhatIsInside(t *testing.T) {
	r, _ := tree(t)
	fd, err := r.open("src/main.go", unix.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("src/main.go: %v", err)
	}
	unix.Close(fd)
}

func TestRootRefusesEveryEscape(t *testing.T) {
	r, _ := tree(t)
	for _, p := range []string{
		"../outside/secret", // up and out
		"src/../../outside/secret",
		"/etc/passwd", // absolute
		"away/secret", // through a symlink that leaves
		"away",        // the symlink itself
		"rooted",      // an absolute symlink, even one aimed back inside
	} {
		if fd, err := r.open(p, unix.O_RDONLY, 0); err == nil {
			unix.Close(fd)
			t.Errorf("%s opened; it is outside the workspace", p)
		}
		if _, err := r.abs(p); err == nil {
			t.Errorf("%s resolved; it is outside the workspace", p)
		}
	}
}

func TestRootFollowsASymlinkThatStaysInside(t *testing.T) {
	r, _ := tree(t)
	name, err := r.abs("here")
	if err != nil {
		t.Fatalf("here: %v", err)
	}
	if want := r.name + "/src"; name != want {
		t.Fatalf("got %q want %q", name, want)
	}
}

func TestRootAbsOfTheWorkspaceItself(t *testing.T) {
	r, _ := tree(t)
	name, err := r.abs("")
	if err != nil {
		t.Fatalf("empty path: %v", err)
	}
	if name != r.name {
		t.Fatalf("got %q want %q", name, r.name)
	}
}

func TestRootParentIsInsideToo(t *testing.T) {
	r, _ := tree(t)
	fd, base, err := r.parent("src/main.go")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	unix.Close(fd)
	if base != "main.go" {
		t.Fatalf("got base %q", base)
	}
	if _, _, err := r.parent("../outside/secret"); err == nil {
		t.Fatal("the parent of a path outside the workspace resolved")
	}
}

// A path is resolved by the kernel for a write as well as for a read, so one
// string cannot mean two places. away is a symlink out of the workspace, so
// "away/../f" leaves it even though the string alone looks like it stays.
func TestRootParentRefusesWhatTheKernelRefuses(t *testing.T) {
	r, _ := tree(t)
	for _, p := range []string{
		"away/../escaped",
		"../outside/planted",
		"/tmp/planted",
		"src/../../outside/planted",
	} {
		if fd, _, err := r.parent(p); err == nil {
			unix.Close(fd)
			t.Errorf("the parent of %s resolved; a read of it is refused", p)
		}
	}
}

// RESOLVE_BENEATH refuses a ".." that escapes, not every "..".
func TestRootTakesADotDotThatStaysInside(t *testing.T) {
	r, _ := tree(t)
	fd, err := r.open("src/../src/main.go", unix.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("src/../src/main.go: %v", err)
	}
	unix.Close(fd)

	fd, base, err := r.parent("src/../src/main.go")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	unix.Close(fd)
	if base != "main.go" {
		t.Fatalf("got base %q", base)
	}
}

// A write or a patch holds the directory of each file it names while it runs,
// and a command may start meanwhile. That descriptor closes on exec like every
// other the daemon opens, the workspace's own included.
func TestRootParentClosesOnExec(t *testing.T) {
	r, _ := tree(t)
	for _, p := range []string{"top.go", "src/main.go"} {
		fd, _, err := r.parent(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		unix.Close(fd)
		if err != nil {
			t.Fatal(err)
		}
		if flags&unix.FD_CLOEXEC == 0 {
			t.Errorf("the directory of %s is held without FD_CLOEXEC", p)
		}
	}
}
