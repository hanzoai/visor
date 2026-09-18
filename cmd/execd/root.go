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
	"fmt"
	"os"
	"path"
	"strings"

	"golang.org/x/sys/unix"
)

// root is the workspace. Every path in a request is resolved relative to it
// with RESOLVE_BENEATH, so the kernel — not a string check — rejects "..",
// an absolute path, and a symlink that leaves the tree.
type root struct {
	fd   int    // O_PATH descriptor of the workspace directory
	name string // its canonical absolute path
}

const resolve = unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS

func openRoot(dir string) (*root, error) {
	fd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open workspace %s: %w", dir, err)
	}
	name, err := readFd(fd)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &root{fd: fd, name: name}, nil
}

func (r *root) close() error { return unix.Close(r.fd) }

// open resolves p beneath the workspace and returns an open descriptor.
func (r *root) open(p string, flags int, mode uint32) (int, error) {
	return r.openAt(r.fd, p, flags, mode)
}

func (r *root) openAt(dirfd int, p string, flags int, mode uint32) (int, error) {
	if p == "" {
		p = "."
	}
	how := &unix.OpenHow{
		Flags:   uint64(flags) | unix.O_CLOEXEC,
		Mode:    uint64(mode),
		Resolve: resolve,
	}
	fd, err := unix.Openat2(dirfd, p, how)
	if err != nil {
		return -1, fmt.Errorf("%s: %w", p, err)
	}
	return fd, nil
}

// file resolves p and hands back an *os.File that owns the descriptor.
func (r *root) file(p string, flags int, mode uint32) (*os.File, error) {
	fd, err := r.open(p, flags, mode)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path.Join(r.name, p)), nil
}

// abs resolves p beneath the workspace and returns its canonical absolute
// path. Used where the kernel interface takes a path and not a descriptor:
// a child's working directory, an inotify watch.
func (r *root) abs(p string) (string, error) {
	fd, err := r.open(p, unix.O_PATH, 0)
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	name, err := readFd(fd)
	if err != nil {
		return "", err
	}
	if name != r.name && !strings.HasPrefix(name, r.name+"/") {
		return "", fmt.Errorf("%s: resolves outside the workspace", p)
	}
	return name, nil
}

// parent resolves p's directory beneath the workspace and returns its
// descriptor together with p's last component, which is what an atomic
// create-then-rename needs.
func (r *root) parent(p string) (int, string, error) {
	dir, base := path.Split(path.Clean(p))
	if base == "" || base == "." || base == ".." {
		return -1, "", fmt.Errorf("%s: not a file name", p)
	}
	if dir == "" {
		fd, err := unix.Dup(r.fd)
		if err != nil {
			return -1, "", err
		}
		return fd, base, nil
	}
	fd, err := r.open(strings.TrimSuffix(dir, "/"), unix.O_PATH|unix.O_DIRECTORY, 0)
	if err != nil {
		return -1, "", err
	}
	return fd, base, nil
}

func readFd(fd int) (string, error) {
	name, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return "", fmt.Errorf("read /proc/self/fd/%d: %w", fd, err)
	}
	return name, nil
}
