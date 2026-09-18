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

// execd runs inside a visor sandbox and does what a coding agent asks of an
// operating system: run a program, drive a pty, read and write files, watch
// them, apply a diff, run git. It knows nothing about models, sessions or
// plans — those live outside the sandbox and reach it as actions.
//
// It serves ZAP frames on one descriptor it inherits already connected: FD 3,
// one end of a SOCK_SEQPACKET socketpair the host made. There is no listen,
// no connect and no path in a filesystem. See docs/execd.md.
//go:build linux

package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// hostFd is the descriptor the host connects before exec: one end of a
// socketpair, already connected, inherited across the exec.
const hostFd = 3

func main() {
	workspace := flag.String("workspace", "", "directory every path in a request is resolved beneath")
	fd := flag.Int("fd", hostFd, "inherited connected SOCK_SEQPACKET descriptor")
	flag.Parse()

	if *workspace == "" {
		die(fmt.Errorf("-workspace is required"))
	}
	if err := checkSocket(*fd); err != nil {
		die(err)
	}
	r, err := openRoot(*workspace)
	if err != nil {
		die(err)
	}
	defer r.close()

	if err := newDaemon(*fd, r).serve(); err != nil {
		die(err)
	}
}

// checkSocket refuses anything that is not the contract: a connected
// sequenced-packet socket. It also asks the kernel for room for a whole
// frame, which is a request and not a requirement.
func checkSocket(fd int) error {
	kind, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil {
		return fmt.Errorf("descriptor %d is not a socket: %w", fd, err)
	}
	if kind != unix.SOCK_SEQPACKET {
		return fmt.Errorf("descriptor %d is socket type %d, not SOCK_SEQPACKET", fd, kind)
	}
	if _, err := unix.Getpeername(fd); err != nil {
		return fmt.Errorf("descriptor %d is not connected: %w", fd, err)
	}
	unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, 4*frameMax)
	unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, 4*frameMax)
	return nil
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "execd: %v\n", err)
	os.Exit(1)
}
