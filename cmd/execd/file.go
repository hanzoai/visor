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
	"io"
	"os"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

// stage names the temporary file an atomic write goes through. The action id
// makes it unique: no two live actions share one.
func stage(base string, id uint64) string {
	return fmt.Sprintf(".%s.execd%d", base, id)
}

const (
	// readMax bounds one read. A larger file is read range by range.
	readMax = 96 << 10
	// listMax bounds one listing. A larger directory is listed page by page
	// with off as the first index and length as the count.
	listMax = 4096
	// fileMode is what a created file gets when the request says nothing.
	fileMode = 0o644
)

func (d *daemon) readFile(r *req) *rep {
	f, err := d.root.file(r.path, unix.O_RDONLY, 0)
	if err != nil {
		return fail(r, err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return fail(r, err)
	}
	if st.IsDir() {
		return fail(r, fmt.Errorf("%s: is a directory", r.path))
	}
	if r.off < 0 {
		return fail(r, fmt.Errorf("offset %d is negative", r.off))
	}

	n := int64(r.length)
	if n == 0 {
		n = st.Size() - r.off
	}
	if n < 0 {
		n = 0
	}
	if n > readMax {
		return fail(r, fmt.Errorf("%d bytes is over the %d byte read; ask for a range", n, readMax))
	}

	buf := make([]byte, n)
	got, err := f.ReadAt(buf, r.off)
	if err != nil && !errors.Is(err, io.EOF) {
		return fail(r, err)
	}
	return &rep{
		data:  buf[:got],
		size:  uint64(st.Size()),
		mode:  uint32(st.Mode().Perm()),
		mtime: st.ModTime().UnixNano(),
	}
}

// writeFile replaces a file's contents, and does it by rename so a reader
// sees either the old bytes or the new ones.
func (d *daemon) writeFile(r *req) *rep {
	mode := r.mode
	if mode == 0 {
		mode = fileMode
	}
	if err := d.place(r.path, r.data, mode, r.id); err != nil {
		return fail(r, err)
	}
	return &rep{size: uint64(len(r.data)), mode: mode, path: r.path}
}

// place writes data to a temporary file beside path and renames it over the
// top. A reader sees the old bytes or the new ones and never a half-written
// file. There is no fsync: the workspace is served by the host, so making
// the bytes durable is the host's job, not a syscall inside the sandbox.
func (d *daemon) place(name string, data []byte, mode uint32, id uint64) error {
	dirfd, base, err := d.root.parent(name)
	if err != nil {
		return err
	}
	defer unix.Close(dirfd)

	tmp := stage(base, id)
	unix.Unlinkat(dirfd, tmp, 0)
	fd, err := d.root.openAt(dirfd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, mode)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		unix.Unlinkat(dirfd, tmp, 0)
		return err
	}
	if err := f.Close(); err != nil {
		unix.Unlinkat(dirfd, tmp, 0)
		return err
	}
	if err := unix.Renameat(dirfd, tmp, dirfd, base); err != nil {
		unix.Unlinkat(dirfd, tmp, 0)
		return fmt.Errorf("rename over %s: %w", name, err)
	}
	return nil
}

func (d *daemon) statPath(r *req) *rep {
	fd, err := d.root.open(r.path, unix.O_PATH, 0)
	if err != nil {
		return fail(r, err)
	}
	defer unix.Close(fd)

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fail(r, err)
	}
	reply := &rep{
		size:  uint64(st.Size),
		mode:  st.Mode,
		mtime: st.Mtim.Nano(),
		path:  r.path,
	}
	if st.Mode&unix.S_IFMT == unix.S_IFDIR {
		reply.flags |= flagDir
	}
	return reply
}

func (d *daemon) listDir(r *req) *rep {
	f, err := d.root.file(r.path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return fail(r, err)
	}
	defer f.Close()

	all, err := f.ReadDir(-1)
	if err != nil {
		return fail(r, err)
	}
	// Directory order is whatever the filesystem hands back, so sort: a page
	// is only meaningful against a stable order.
	slices.SortFunc(all, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	if r.off < 0 || r.off > int64(len(all)) {
		return fail(r, fmt.Errorf("index %d is outside a directory of %d entries", r.off, len(all)))
	}
	want := int(r.length)
	if want == 0 || want > listMax {
		want = listMax
	}
	page := all[r.off:]
	if len(page) > want {
		page = page[:want]
	}

	reply := &rep{size: uint64(len(all)), path: r.path}
	for _, de := range page {
		e := entry{name: de.Name()}
		if de.IsDir() {
			e.flags |= flagDir
		}
		if info, err := de.Info(); err == nil {
			e.size = uint64(info.Size())
			e.mode = uint32(info.Mode().Perm())
			e.mtime = info.ModTime().UnixNano()
		}
		reply.entries = append(reply.entries, e)
	}
	return reply
}
