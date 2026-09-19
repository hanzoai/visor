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
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"sync"

	zap "github.com/zap-proto/go"
	"golang.org/x/sys/unix"
)

const (
	// ledgerMax is how many completed actions keep their result.
	ledgerMax = 1024
	// ledgerRoom is how many bytes those results may take together.
	ledgerRoom = 8 << 20
)

// daemon serves one connected descriptor. It holds a workspace, the action
// ledger, and the live pty and watch handles.
type daemon struct {
	root   *root
	ledger *ledger

	fd   int
	out  chan []byte
	end  chan struct{} // closed when no more frames will be pushed
	gone chan struct{} // closed when the writer has stopped

	mu      sync.Mutex
	handles map[uint64]*handle
	next    uint64

	acts  sync.WaitGroup // in-flight requests
	pumps sync.WaitGroup // pty and watch goroutines
}

// handle is a pty or a watch: something that outlives the request that made
// it and pushes frames under that request's action id.
type handle struct {
	id   uint64
	kind uint8
	once sync.Once

	pty *os.File  // opSpawn
	cmd *exec.Cmd // opSpawn

	nfy  int           // opWatch: the inotify descriptor
	stop [2]int        // opWatch: the pipe that wakes its pump
	done chan struct{} // opWatch: closed when its pump has returned
}

// newDaemon takes the descriptor over. It arrives inheritable, since that is
// how it crossed the exec that started execd, and it crosses no other: a child
// that held it could read requests meant for the daemon and answer in its
// name.
func newDaemon(fd int, r *root) *daemon {
	unix.CloseOnExec(fd)
	return &daemon{
		root:    r,
		ledger:  newLedger(ledgerMax, ledgerRoom),
		fd:      fd,
		out:     make(chan []byte, 64),
		end:     make(chan struct{}),
		gone:    make(chan struct{}),
		handles: make(map[uint64]*handle),
	}
}

// serve reads requests until the host closes the descriptor. Each request is
// carried out on its own goroutine, so a long exec does not hold up a pty
// write; every reply names the action id it answers.
func (d *daemon) serve() error {
	go d.writer()

	buf := make([]byte, frameMax)
	var err error
	for {
		n, _, rerr := unix.Recvfrom(d.fd, buf, unix.MSG_TRUNC)
		if rerr == unix.EINTR {
			continue
		}
		if rerr != nil {
			err = fmt.Errorf("recv: %w", rerr)
			break
		}
		if n == 0 {
			// A zero-length packet is a packet, so the socket and not the
			// length says whether the host is gone.
			gone, perr := hangup(d.fd)
			if perr != nil {
				err = fmt.Errorf("poll: %w", perr)
				break
			}
			if gone {
				break // the host closed its end
			}
			continue // an empty packet asks for nothing
		}
		if n > len(buf) {
			d.refuseOversize(buf, n)
			continue
		}
		frame := make([]byte, n)
		copy(frame, buf[:n])
		d.acts.Add(1)
		go func() {
			defer d.acts.Done()
			d.dispatch(frame)
		}()
	}

	d.acts.Wait()
	d.shut()
	d.pumps.Wait()
	close(d.end)
	<-d.gone
	return err
}

// hangup reports whether the host has closed its end or shut down its side of
// it. A sequenced-packet socket delivers a zero-length packet as readable, so
// this is what tells an empty packet from end of file.
func hangup(fd int) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLRDHUP}}
	for {
		if _, err := unix.Poll(fds, 0); err != nil {
			if err == unix.EINTR {
				continue
			}
			return false, err
		}
		return fds[0].Revents&(unix.POLLRDHUP|unix.POLLHUP) != 0, nil
	}
}

// writer owns the descriptor's send side: one frame per packet, in order.
func (d *daemon) writer() {
	defer close(d.gone)
	for {
		select {
		case frame := <-d.out:
			if _, err := unix.Write(d.fd, frame); err != nil {
				return
			}
		case <-d.end:
			for {
				select {
				case frame := <-d.out:
					if _, err := unix.Write(d.fd, frame); err != nil {
						return
					}
				default:
					return
				}
			}
		}
	}
}

// push queues one frame. It gives up if the writer has stopped, so a pty
// pump can never block on a dead descriptor.
func (d *daemon) push(frame []byte) {
	select {
	case d.out <- frame:
	case <-d.gone:
	}
}

func (d *daemon) send(r *rep) {
	d.push(r.encode())
}

// refuseOversize answers a request too large to receive. The op and the id sit
// at fixed offsets, so the truncated head is enough to name both.
func (d *daemon) refuseOversize(head []byte, n int) {
	r := &req{}
	if len(head) >= zap.HeaderSize {
		at := int(binary.LittleEndian.Uint32(head[8:12]))
		if off := at + reqOp; off >= 0 && off < len(head) {
			r.op = head[off]
		}
		if off := at + reqID; off >= 0 && off+8 <= len(head) {
			r.id = binary.LittleEndian.Uint64(head[off:])
		}
	}
	d.send(fail(r, fmt.Errorf("request is %d bytes, over the %d byte frame", n, frameMax)))
}

func (d *daemon) dispatch(frame []byte) {
	r, err := decodeReq(frame)
	if err != nil {
		d.send(fail(&req{}, fmt.Errorf("decode: %w", err)))
		return
	}
	recorded, err := d.ledger.begin(r.id)
	if err != nil {
		d.send(fail(r, err))
		return
	}
	if recorded != nil {
		d.push(recorded)
		return
	}
	reply := d.act(r)
	reply.op, reply.kind, reply.id = r.op, kindReply, r.id
	out := reply.encode()
	if len(out) > frameMax {
		big := fail(r, fmt.Errorf("reply is %d bytes, over the %d byte frame", len(out), frameMax))
		// The action still happened if it happened; only the answer is gone.
		big.refused = reply.refused
		out = big.encode()
	}
	if reply.refused {
		// Nothing happened, so there is nothing to replay and a repeat of the
		// id runs.
		d.ledger.drop(r.id)
	} else {
		d.ledger.finish(r.id, out)
	}
	d.push(out)
}

func (d *daemon) act(r *req) *rep {
	switch r.op {
	case opExec:
		return d.exec(r)
	case opSpawn:
		return d.spawn(r)
	case opPtyWrite:
		return d.ptyWrite(r)
	case opResize:
		return d.resize(r)
	case opKill:
		return d.kill(r)
	case opRead:
		return d.readFile(r)
	case opWrite:
		return d.writeFile(r)
	case opStat:
		return d.statPath(r)
	case opList:
		return d.listDir(r)
	case opWatch:
		return d.watch(r)
	case opPatch:
		return d.patch(r)
	case opGit:
		return d.git(r)
	}
	return fail(r, fmt.Errorf("unknown op %d", r.op))
}

// add registers a pty or watch handle and returns its number.
func (d *daemon) add(h *handle) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next++
	d.handles[d.next] = h
	return d.next
}

// get looks a handle up. kind zero accepts either kind.
func (d *daemon) get(n uint64, kind uint8) (*handle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.handles[n]
	if !ok {
		return nil, fmt.Errorf("no handle %d", n)
	}
	if kind != 0 && h.kind != kind {
		return nil, fmt.Errorf("handle %d is a %s, not a %s", n, opName(h.kind), opName(kind))
	}
	return h, nil
}

func (d *daemon) forget(n uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.handles, n)
}

// shut closes every live handle, which ends its pump goroutine.
func (d *daemon) shut() {
	d.mu.Lock()
	hs := make([]*handle, 0, len(d.handles))
	for n, h := range d.handles {
		hs = append(hs, h)
		delete(d.handles, n)
	}
	d.mu.Unlock()
	for _, h := range hs {
		h.shut()
	}
}

// shut closes a handle once. A pty's process is killed, which ends its read
// loop; a watch's pump is woken through its pipe and the descriptors are
// closed only after that loop has returned.
func (h *handle) shut() {
	h.once.Do(func() {
		switch h.kind {
		case opSpawn:
			if h.cmd != nil && h.cmd.Process != nil {
				unix.Kill(-h.cmd.Process.Pid, unix.SIGKILL)
			}
		case opWatch:
			unix.Write(h.stop[1], []byte{0})
			<-h.done
			unix.Close(h.nfy)
			unix.Close(h.stop[0])
			unix.Close(h.stop[1])
		}
	})
}
