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

	zap "github.com/zap-proto/go"
)

// Ops. One byte on the wire, stable forever: a number is never reused.
const (
	opExec     uint8 = 1  // run argv or command to completion
	opSpawn    uint8 = 2  // start a process on a pty, return a handle
	opPtyWrite uint8 = 3  // write bytes to a pty
	opResize   uint8 = 4  // set a pty window size
	opKill     uint8 = 5  // signal a pty process, or close a watch
	opRead     uint8 = 6  // read a byte range of a file
	opWrite    uint8 = 7  // replace a file's contents
	opStat     uint8 = 8  // one path's size, mode, mtime
	opList     uint8 = 9  // one directory's entries
	opWatch    uint8 = 10 // inotify on a path, return a handle
	opPatch    uint8 = 11 // apply a unified diff
	opGit      uint8 = 12 // run git in the workspace
)

func opName(op uint8) string {
	switch op {
	case opExec:
		return "exec"
	case opSpawn:
		return "spawn"
	case opPtyWrite:
		return "pty.write"
	case opResize:
		return "resize"
	case opKill:
		return "kill"
	case opRead:
		return "read"
	case opWrite:
		return "write"
	case opStat:
		return "stat"
	case opList:
		return "list"
	case opWatch:
		return "watch"
	case opPatch:
		return "patch"
	case opGit:
		return "git"
	}
	return fmt.Sprintf("op(%d)", op)
}

// Reply kinds. A reply answers a request; the other kinds are pushed by a
// handle the host already holds, and are never recorded in the ledger.
const (
	kindReply uint8 = 0
	kindData  uint8 = 1 // pty output
	kindEvent uint8 = 2 // watch event
	kindExit  uint8 = 3 // the spawned process ended
)

// Flag bits in rep.flags.
const (
	flagDir uint32 = 1 << 0
)

// frameMax bounds one packet. A request over it cannot be received and a
// reply over it is refused with the size that would have been needed, so a
// big file is read in ranges rather than truncated in silence.
const frameMax = 128 << 10

// Request field offsets. Fixed section, little-endian, zero-copy read.
const (
	reqOp      = 0
	reqCols    = 4
	reqRows    = 8
	reqSignal  = 12
	reqID      = 16
	reqHandle  = 24
	reqTimeout = 32
	reqOff     = 40
	reqLen     = 48
	reqMode    = 52
	reqPath    = 56
	reqData    = 64
	reqDir     = 72
	reqArgv    = 80
	reqEnv     = 88
	reqCommand = 96
	reqSize    = 104
)

// Reply field offsets.
const (
	repOp      = 0
	repKind    = 1
	repExit    = 4
	repID      = 8
	repHandle  = 16
	repSize    = 24
	repMode    = 32
	repFlags   = 36
	repMtime   = 40
	repData    = 48
	repLog     = 56
	repErr     = 64
	repPath    = 72
	repEntries = 80
	repMask    = 88
	repRepSize = 96
)

// Directory entry field offsets, inside one element of rep.entries.
const (
	entSize  = 0
	entMode  = 8
	entFlags = 12
	entMtime = 16
	entName  = 24
	entTotal = 32
)

// req is one action. Every request carries an id; the reply names it.
type req struct {
	op      uint8
	id      uint64
	handle  uint64
	cols    uint32
	rows    uint32
	signal  int32
	timeout uint64 // milliseconds; 0 means no deadline
	off     int64
	length  uint32
	mode    uint32
	path    string
	data    []byte
	dir     string
	argv    []string
	env     []string
	command string
}

// rep answers one action, or pushes output under a handle's action id.
type rep struct {
	op      uint8
	kind    uint8
	id      uint64
	handle  uint64
	exit    int32
	size    uint64
	mode    uint32
	flags   uint32
	mtime   int64
	mask    uint32
	data    []byte
	log     []byte
	err     string
	path    string
	entries []entry
}

// entry is one name in a directory listing.
type entry struct {
	name  string
	size  uint64
	mode  uint32
	flags uint32
	mtime int64
}

func (r *req) encode() []byte {
	b := zap.NewBuilder(512 + len(r.data))
	argvOff, argvLen := texts(b, r.argv)
	envOff, envLen := texts(b, r.env)

	ob := b.StartObject(reqSize)
	ob.SetUint8(reqOp, r.op)
	ob.SetUint32(reqCols, r.cols)
	ob.SetUint32(reqRows, r.rows)
	ob.SetInt32(reqSignal, r.signal)
	ob.SetUint64(reqID, r.id)
	ob.SetUint64(reqHandle, r.handle)
	ob.SetUint64(reqTimeout, r.timeout)
	ob.SetInt64(reqOff, r.off)
	ob.SetUint32(reqLen, r.length)
	ob.SetUint32(reqMode, r.mode)
	ob.SetText(reqPath, r.path)
	ob.SetBytes(reqData, r.data)
	ob.SetText(reqDir, r.dir)
	ob.SetList(reqArgv, argvOff, argvLen)
	ob.SetList(reqEnv, envOff, envLen)
	ob.SetText(reqCommand, r.command)
	ob.FinishAsRoot()
	return b.Finish()
}

func decodeReq(buf []byte) (*req, error) {
	m, err := zap.Parse(buf)
	if err != nil {
		return nil, err
	}
	o := m.Root()
	if o.IsNull() {
		return nil, fmt.Errorf("request has no root")
	}
	return &req{
		op:      o.Uint8(reqOp),
		cols:    o.Uint32(reqCols),
		rows:    o.Uint32(reqRows),
		signal:  o.Int32(reqSignal),
		id:      o.Uint64(reqID),
		handle:  o.Uint64(reqHandle),
		timeout: o.Uint64(reqTimeout),
		off:     o.Int64(reqOff),
		length:  o.Uint32(reqLen),
		mode:    o.Uint32(reqMode),
		path:    string(o.Bytes(reqPath)),
		data:    o.Bytes(reqData),
		dir:     string(o.Bytes(reqDir)),
		argv:    readTexts(o.List(reqArgv)),
		env:     readTexts(o.List(reqEnv)),
		command: string(o.Bytes(reqCommand)),
	}, nil
}

func (r *rep) encode() []byte {
	b := zap.NewBuilder(512 + len(r.data) + len(r.log))
	entOff, entLen := entries(b, r.entries)

	ob := b.StartObject(repRepSize)
	ob.SetUint8(repOp, r.op)
	ob.SetUint8(repKind, r.kind)
	ob.SetInt32(repExit, r.exit)
	ob.SetUint64(repID, r.id)
	ob.SetUint64(repHandle, r.handle)
	ob.SetUint64(repSize, r.size)
	ob.SetUint32(repMode, r.mode)
	ob.SetUint32(repFlags, r.flags)
	ob.SetInt64(repMtime, r.mtime)
	ob.SetUint32(repMask, r.mask)
	ob.SetBytes(repData, r.data)
	ob.SetBytes(repLog, r.log)
	ob.SetText(repErr, r.err)
	ob.SetText(repPath, r.path)
	ob.SetList(repEntries, entOff, entLen)
	ob.FinishAsRoot()
	return b.Finish()
}

func decodeRep(buf []byte) (*rep, error) {
	m, err := zap.Parse(buf)
	if err != nil {
		return nil, err
	}
	o := m.Root()
	if o.IsNull() {
		return nil, fmt.Errorf("reply has no root")
	}
	r := &rep{
		op:     o.Uint8(repOp),
		kind:   o.Uint8(repKind),
		exit:   o.Int32(repExit),
		id:     o.Uint64(repID),
		handle: o.Uint64(repHandle),
		size:   o.Uint64(repSize),
		mode:   o.Uint32(repMode),
		flags:  o.Uint32(repFlags),
		mtime:  o.Int64(repMtime),
		mask:   o.Uint32(repMask),
		data:   o.Bytes(repData),
		log:    o.Bytes(repLog),
		err:    string(o.Bytes(repErr)),
		path:   string(o.Bytes(repPath)),
	}
	l := o.List(repEntries)
	for i := 0; i < l.Len(); i++ {
		e := l.ObjectAt(i)
		if e.IsNull() {
			return nil, fmt.Errorf("entry %d does not parse", i)
		}
		r.entries = append(r.entries, entry{
			name:  string(e.Bytes(entName)),
			size:  e.Uint64(entSize),
			mode:  e.Uint32(entMode),
			flags: e.Uint32(entFlags),
			mtime: e.Int64(entMtime),
		})
	}
	return r, nil
}

// texts lays a list of strings down as length-prefixed elements and returns
// its offset and element count.
func texts(b *zap.Builder, ss []string) (int, int) {
	if len(ss) == 0 {
		return 0, 0
	}
	lb := b.StartList(0)
	for _, s := range ss {
		lb.AddUint32(uint32(len(s)))
		lb.AddBytes([]byte(s))
	}
	off, _ := lb.Finish()
	return off, len(ss)
}

func readTexts(l zap.List) []string {
	n := l.Len()
	if n == 0 {
		return nil
	}
	ss := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ss = append(ss, string(l.BytesAt(i)))
	}
	return ss
}

// entries lays each directory entry down as its own ZAP message inside a
// length-prefixed element, which is what List.ObjectAt reads.
func entries(b *zap.Builder, es []entry) (int, int) {
	if len(es) == 0 {
		return 0, 0
	}
	lb := b.StartList(0)
	for _, e := range es {
		eb := zap.NewBuilder(64 + len(e.name))
		ob := eb.StartObject(entTotal)
		ob.SetUint64(entSize, e.size)
		ob.SetUint32(entMode, e.mode)
		ob.SetUint32(entFlags, e.flags)
		ob.SetInt64(entMtime, e.mtime)
		ob.SetText(entName, e.name)
		ob.FinishAsRoot()
		buf := eb.Finish()
		lb.AddUint32(uint32(len(buf)))
		lb.AddBytes(buf)
	}
	off, _ := lb.Finish()
	return off, len(es)
}

// fail is the reply to a request that could not be carried out. The reason
// travels with the action id; nothing is ever answered with a false success.
func fail(r *req, err error) *rep {
	return &rep{op: r.op, kind: kindReply, id: r.id, exit: -1, err: err.Error()}
}
