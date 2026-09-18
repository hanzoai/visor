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
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// outMax bounds each of a child's two output streams in one reply. Past it
// the reply says how much was dropped; it never pretends the output is whole.
const outMax = 48 << 10

// shell is what a command string is handed to.
var shell = []string{"/bin/sh", "-c"}

func (d *daemon) exec(r *req) *rep {
	argv, err := argvOf(r)
	if err != nil {
		return fail(r, err)
	}
	return d.run(r, argv, nil)
}

// argvOf settles what a request asked to run: argv, or a command string for
// a shell. Both, or neither, is refused.
func argvOf(r *req) ([]string, error) {
	switch {
	case r.command != "" && len(r.argv) > 0:
		return nil, errors.New("argv and command are both set; give one")
	case r.command != "":
		return append(append([]string{}, shell...), r.command), nil
	case len(r.argv) > 0:
		return r.argv, nil
	}
	return nil, errors.New("neither argv nor command is set")
}

// run starts a child in the workspace and waits for it. extra is appended to
// the child's environment after the request's own.
func (d *daemon) run(r *req, argv []string, extra []string) *rep {
	dir, err := d.root.abs(r.dir)
	if err != nil {
		return fail(r, err)
	}

	ctx := context.Background()
	stop := func() {}
	if r.timeout > 0 {
		ctx, stop = context.WithTimeout(ctx, time.Duration(r.timeout)*time.Millisecond)
	}
	defer stop()

	env := os.Environ()
	if len(r.env) > 0 {
		env = r.env
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(append([]string{}, env...), extra...)
	if len(r.data) > 0 {
		cmd.Stdin = bytes.NewReader(r.data)
	}
	out := &capped{max: outMax}
	log := &capped{max: outMax}
	cmd.Stdout, cmd.Stderr = out, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != syscall.ESRCH {
			return err
		}
		return nil
	}
	cmd.WaitDelay = time.Second

	werr := cmd.Run()
	reply := &rep{data: out.buf, log: log.buf}

	var exit *exec.ExitError
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		// The deadline is the reason, whatever shape the wait error took.
		reply.exit = -1
		reply.err = fmt.Sprintf("no answer within %dms", r.timeout)
	case werr == nil:
	case errors.As(werr, &exit):
		reply.exit = int32(exit.ExitCode())
		if st, ok := exit.Sys().(syscall.WaitStatus); ok && st.Signaled() {
			reply.err = fmt.Sprintf("killed by %s", st.Signal())
		}
	default:
		return fail(r, fmt.Errorf("%s: %w", argv[0], werr))
	}
	if note := strings.TrimSpace(out.note("stdout") + " " + log.note("stderr")); note != "" {
		if reply.err != "" {
			reply.err += "; "
		}
		reply.err += note
	}
	return reply
}

// capped collects up to max bytes and counts what it had to drop.
type capped struct {
	buf  []byte
	max  int
	over int
}

func (c *capped) Write(p []byte) (int, error) {
	room := c.max - len(c.buf)
	if room > len(p) {
		room = len(p)
	}
	if room > 0 {
		c.buf = append(c.buf, p[:room]...)
	} else {
		room = 0
	}
	c.over += len(p) - room
	return len(p), nil
}

func (c *capped) note(name string) string {
	if c.over == 0 {
		return ""
	}
	return fmt.Sprintf("%s kept %d bytes and dropped %d; redirect it to a file", name, len(c.buf), c.over)
}
