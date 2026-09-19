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
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// ptyRead is how much pty output one pushed frame carries.
const ptyRead = 16 << 10

// spawn starts a process on a pty and answers with a handle. Output arrives
// afterwards as kindData frames under the same action id, and the process's
// end as one kindExit frame.
func (d *daemon) spawn(r *req) *rep {
	argv, err := argvOf(r)
	if err != nil {
		return fail(r, err)
	}
	dir, err := d.root.abs(r.dir)
	if err != nil {
		return fail(r, err)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = environ(r)
	size := &pty.Winsize{Cols: uint16(r.cols), Rows: uint16(r.rows)}
	if size.Cols == 0 {
		size.Cols = 80
	}
	if size.Rows == 0 {
		size.Rows = 24
	}
	f, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return fail(r, fmt.Errorf("%s: %w", argv[0], err))
	}

	h := &handle{id: r.id, kind: opSpawn, pty: f, cmd: cmd, nfy: -1}
	n := d.add(h)
	d.pumps.Add(1)
	go d.pumpPty(h, n)
	return &rep{handle: n}
}

// pumpPty pushes a pty's output, then the process's exit.
func (d *daemon) pumpPty(h *handle, n uint64) {
	defer d.pumps.Done()
	buf := make([]byte, ptyRead)
	for {
		got, err := h.pty.Read(buf)
		if got > 0 {
			out := make([]byte, got)
			copy(out, buf[:got])
			d.send(&rep{op: opSpawn, kind: kindData, id: h.id, handle: n, data: out})
		}
		if err != nil {
			break
		}
	}

	end := &rep{op: opSpawn, kind: kindExit, id: h.id, handle: n}
	err := h.cmd.Wait()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		end.exit = int32(exit.ExitCode())
		if st, ok := exit.Sys().(syscall.WaitStatus); ok && st.Signaled() {
			end.err = fmt.Sprintf("killed by %s", st.Signal())
		}
	default:
		end.exit = -1
		end.err = err.Error()
	}
	h.pty.Close()
	d.forget(n)
	d.send(end)
}

func (d *daemon) ptyWrite(r *req) *rep {
	h, err := d.get(r.handle, opSpawn)
	if err != nil {
		return fail(r, err)
	}
	n, err := h.pty.Write(r.data)
	if err != nil && n == 0 {
		return fail(r, err)
	}
	reply := &rep{handle: r.handle, size: uint64(n)}
	if err != nil {
		// Some of it went in, so the write happened: say how much, and why
		// the rest did not.
		reply.err = err.Error()
	}
	return reply
}

func (d *daemon) resize(r *req) *rep {
	h, err := d.get(r.handle, opSpawn)
	if err != nil {
		return fail(r, err)
	}
	if r.cols == 0 || r.rows == 0 {
		return fail(r, fmt.Errorf("a window is %dx%d", r.cols, r.rows))
	}
	size := &pty.Winsize{Cols: uint16(r.cols), Rows: uint16(r.rows)}
	if err := pty.Setsize(h.pty, size); err != nil {
		return fail(r, err)
	}
	return &rep{handle: r.handle}
}

// kill signals a pty's process group, or closes a watch. The signal
// defaults to SIGKILL; a watch ignores it.
func (d *daemon) kill(r *req) *rep {
	h, err := d.get(r.handle, 0)
	if err != nil {
		return fail(r, err)
	}
	switch h.kind {
	case opSpawn:
		sig := syscall.Signal(r.signal)
		if sig == 0 {
			sig = syscall.SIGKILL
		}
		if h.cmd.Process == nil {
			return fail(r, fmt.Errorf("handle %d has no process", r.handle))
		}
		if err := syscall.Kill(-h.cmd.Process.Pid, sig); err != nil {
			return fail(r, err)
		}
	case opWatch:
		h.shut()
		d.forget(r.handle)
	}
	return &rep{handle: r.handle}
}
