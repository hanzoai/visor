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
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	zap "github.com/zap-proto/go"
	"golang.org/x/sys/unix"
)

const (
	// readMax bounds one read. A larger file is read range by range.
	readMax = 96 << 10
	// fileMode is what a created file gets when the request says nothing.
	fileMode = 0o644
	// perm masks a mode down to the permission bits, which is what mode means
	// in a request and in every reply that carries one.
	perm = 0o777
)

// scratch names a file a request cannot name: the suffix is random, and the
// O_EXCL or the linkat that follows proves the name was free. A file the
// workspace already holds is never in the way of a write.
func scratch() (string, error) {
	var n [8]byte
	if _, err := rand.Read(n[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf(".execd%x", n), nil
}

// stage creates a file in dirfd under such a name, to be renamed over the one
// the request asked for once it holds the whole of the new contents.
func (d *daemon) stage(dirfd int, mode uint32) (string, int, error) {
	for try := 0; try < 4; try++ {
		name, err := scratch()
		if err != nil {
			return "", -1, err
		}
		fd, err := d.root.openAt(dirfd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, mode)
		if err == nil {
			return name, fd, nil
		}
		if !errors.Is(err, unix.EEXIST) {
			return "", -1, err
		}
	}
	return "", -1, errors.New("no free name for a staged file")
}

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
	if r.off > st.Size() {
		return fail(r, fmt.Errorf("offset %d is past the end of a %d byte file", r.off, st.Size()))
	}

	n := int64(r.length)
	if n == 0 || n > st.Size()-r.off {
		n = st.Size() - r.off
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
	mode, err := d.place(r.path, r.data, r.mode)
	if err != nil {
		return fail(r, err)
	}
	return &rep{size: uint64(len(r.data)), mode: mode, path: r.path}
}

// place writes data to a staged file beside path and renames it over the top.
// A reader sees the old bytes or the new ones and never a half-written file.
// There is no fsync: the workspace is served by the host, so making the bytes
// durable is the host's job, not a syscall inside the sandbox.
//
// mode zero keeps the mode the file has, and is fileMode for a file that is
// not there yet. The mode is set rather than left to the umask, and the mode
// the file ends up with is returned, so a reply never names a mode that is
// not on disk.
func (d *daemon) place(name string, data []byte, mode uint32) (uint32, error) {
	dirfd, base, err := d.root.parent(name)
	if err != nil {
		return 0, err
	}
	defer unix.Close(dirfd)

	mode &= perm
	if mode == 0 {
		mode = fileMode
		var st unix.Stat_t
		if err := unix.Fstatat(dirfd, base, &st, 0); err == nil {
			mode = st.Mode & perm
		}
	}
	tmp, fd, err := d.stage(dirfd, mode)
	if err != nil {
		return 0, err
	}
	f := os.NewFile(uintptr(fd), tmp)
	got, err := settle(f, data, mode)
	if err != nil {
		unix.Unlinkat(dirfd, tmp, 0)
		return 0, err
	}
	if err := unix.Renameat(dirfd, tmp, dirfd, base); err != nil {
		unix.Unlinkat(dirfd, tmp, 0)
		return 0, fmt.Errorf("rename over %s: %w", name, err)
	}
	return got, nil
}

// settle fills a staged file, sets its mode past the umask and closes it. It
// reports the mode the file has, which is what the kernel kept of the one
// asked for.
func settle(f *os.File, data []byte, mode uint32) (uint32, error) {
	var st unix.Stat_t
	_, err := f.Write(data)
	if err == nil {
		err = unix.Fchmod(int(f.Fd()), mode)
	}
	if err == nil {
		err = unix.Fstat(int(f.Fd()), &st)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	return st.Mode & perm, nil
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
		mode:  st.Mode & perm,
		mtime: st.Mtim.Nano(),
		path:  r.path,
	}
	if st.Mode&unix.S_IFMT == unix.S_IFDIR {
		reply.flags |= flagDir
	}
	return reply
}

// listDir answers with the entries from off that fit in one frame. size is
// the whole directory's count, so a host pages by advancing off by the number
// of entries it got.
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
	want := len(all)
	if r.length > 0 && int(r.length) < want {
		want = int(r.length)
	}

	reply := &rep{size: uint64(len(all)), path: r.path}
	room := listRoom(r.path)
	for _, de := range all[r.off:] {
		if len(reply.entries) == want {
			break
		}
		room -= entryCost(de.Name())
		if room < 0 {
			break
		}
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
	if len(reply.entries) == 0 && r.off < int64(len(all)) {
		return fail(r, fmt.Errorf("%s: the name at index %d does not fit a frame", r.path, r.off))
	}
	return reply
}

// listRoom is what a listing's entries may take: one frame, less the reply's
// own header, fixed section, path and the padding between them.
func listRoom(path string) int {
	return frameMax - (zap.HeaderSize + repRepSize + len(path) + 2*zap.Alignment)
}

// entryCost is what one entry adds to a listing: a length, and the entry's own
// message — a ZAP header, the fixed section and the name.
func entryCost(name string) int {
	return 4 + zap.HeaderSize + entTotal + len(name)
}
