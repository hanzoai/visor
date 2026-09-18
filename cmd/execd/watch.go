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
	"bytes"
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

// watchMask is what a watch reports: names appearing, leaving, changing, and
// the watched path itself going away.
const watchMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MODIFY |
	unix.IN_CLOSE_WRITE | unix.IN_ATTRIB | unix.IN_MOVED_FROM |
	unix.IN_MOVED_TO | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF

// watch answers with a handle and then pushes one kindEvent frame per
// inotify event under the same action id. One path, not its subtree: the
// host watches the directories it cares about.
func (d *daemon) watch(r *req) *rep {
	name, err := d.root.abs(r.path)
	if err != nil {
		return fail(r, err)
	}
	nfy, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return fail(r, fmt.Errorf("inotify: %w", err))
	}
	if _, err := unix.InotifyAddWatch(nfy, name, watchMask); err != nil {
		unix.Close(nfy)
		return fail(r, fmt.Errorf("watch %s: %w", r.path, err))
	}
	var stop [2]int
	if err := unix.Pipe2(stop[:], unix.O_CLOEXEC); err != nil {
		unix.Close(nfy)
		return fail(r, err)
	}

	h := &handle{id: r.id, kind: opWatch, nfy: nfy, stop: stop, done: make(chan struct{})}
	n := d.add(h)
	d.pumps.Add(1)
	go d.pumpWatch(h, n)
	return &rep{handle: n, path: name}
}

// pumpWatch reads events until the handle is closed. It owns neither
// descriptor's lifetime: shut waits for this loop to return and then closes
// them, so no descriptor is ever closed under a blocked read.
func (d *daemon) pumpWatch(h *handle, n uint64) {
	defer d.pumps.Done()
	defer close(h.done)

	fds := []unix.PollFd{
		{Fd: int32(h.nfy), Events: unix.POLLIN},
		{Fd: int32(h.stop[0]), Events: unix.POLLIN},
	}
	buf := make([]byte, 16<<10)
	for {
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if fds[1].Revents != 0 {
			return
		}
		if fds[0].Revents == 0 {
			continue
		}
		got, err := unix.Read(h.nfy, buf)
		if err == unix.EINTR {
			continue
		}
		if err != nil || got <= 0 {
			return
		}
		for _, e := range events(buf[:got]) {
			e.op, e.kind, e.id, e.handle = opWatch, kindEvent, h.id, n
			d.send(e)
		}
	}
}

// events cuts an inotify read into one reply per event.
func events(buf []byte) []*rep {
	var out []*rep
	for len(buf) >= unix.SizeofInotifyEvent {
		mask := binary.LittleEndian.Uint32(buf[4:])
		size := binary.LittleEndian.Uint32(buf[12:])
		end := unix.SizeofInotifyEvent + int(size)
		if end > len(buf) {
			return out
		}
		name := ""
		if size > 0 {
			raw := buf[unix.SizeofInotifyEvent:end]
			if i := bytes.IndexByte(raw, 0); i >= 0 {
				raw = raw[:i]
			}
			name = string(raw)
		}
		out = append(out, &rep{mask: mask, path: name})
		buf = buf[end:]
	}
	return out
}
