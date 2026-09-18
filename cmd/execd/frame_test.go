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
	"reflect"
	"testing"
)

func TestRequestRoundTrip(t *testing.T) {
	want := &req{
		op:      opExec,
		id:      7821,
		handle:  4,
		cols:    120,
		rows:    40,
		signal:  9,
		timeout: 2500,
		off:     -3,
		length:  4096,
		mode:    0o600,
		path:    "pkg/main.go",
		data:    []byte("on stdin"),
		dir:     "pkg",
		argv:    []string{"cargo", "test", "--", "--nocapture"},
		env:     []string{"PATH=/bin", "RUST_BACKTRACE=1"},
		command: "",
	}
	got, err := decodeReq(want.encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round trip\n want %+v\n  got %+v", want, got)
	}
}

func TestRequestCommandRoundTrip(t *testing.T) {
	want := &req{op: opExec, id: 1, command: "ls -l | wc -l"}
	got, err := decodeReq(want.encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.command != want.command || len(got.argv) != 0 {
		t.Fatalf("got command %q argv %v", got.command, got.argv)
	}
}

func TestReplyRoundTrip(t *testing.T) {
	want := &rep{
		op:     opList,
		kind:   kindReply,
		id:     99,
		handle: 3,
		exit:   -1,
		size:   2,
		mode:   0o644,
		flags:  flagDir,
		mtime:  1750000000123456789,
		mask:   0x200,
		data:   []byte("stdout"),
		log:    []byte("stderr"),
		err:    "no answer within 10ms",
		path:   "src",
		entries: []entry{
			{name: "main.go", size: 12, mode: 0o644, mtime: 5},
			{name: "sub", flags: flagDir, mode: 0o755, mtime: 6},
		},
	}
	got, err := decodeRep(want.encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round trip\n want %+v\n  got %+v", want, got)
	}
}

func TestReplyEmptyFields(t *testing.T) {
	got, err := decodeRep((&rep{op: opStat, id: 5}).encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.id != 5 || got.err != "" || got.data != nil || len(got.entries) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeRefusesGarbage(t *testing.T) {
	if _, err := decodeReq([]byte("not a zap frame at all")); err == nil {
		t.Fatal("a buffer with no ZAP header decoded")
	}
}

// The id has to be readable from a truncated head, which is how an oversize
// request is still answered by the action it names.
func TestIDSitsAtAFixedOffset(t *testing.T) {
	frame := (&req{op: opExec, id: 4242, data: bytes.Repeat([]byte("x"), 1024)}).encode()
	d := &daemon{out: make(chan []byte, 1), gone: make(chan struct{})}
	d.refuseOversize(frame[:256], frameMax+1)
	r, err := decodeRep(<-d.out)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if r.id != 4242 || r.err == "" {
		t.Fatalf("got id %d err %q", r.id, r.err)
	}
}
