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
	"strings"
	"testing"
)

func TestLedgerAnswersACompletedIDFromTheRecord(t *testing.T) {
	l := newLedger(4, 1<<20)
	if frame, err := l.begin(11); err != nil || frame != nil {
		t.Fatalf("first claim: frame %v err %v", frame, err)
	}
	l.finish(11, []byte("the answer"))

	frame, err := l.begin(11)
	if err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if string(frame) != "the answer" {
		t.Fatalf("got %q", frame)
	}
}

func TestLedgerRefusesAnIDStillRunning(t *testing.T) {
	l := newLedger(4, 1<<20)
	l.begin(11)
	_, err := l.begin(11)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("got %v", err)
	}
}

func TestLedgerRefusesAZeroID(t *testing.T) {
	if _, err := newLedger(4, 1<<20).begin(0); err == nil {
		t.Fatal("id zero was accepted")
	}
}

func TestLedgerIsBoundedByBytesToo(t *testing.T) {
	const room = 1024
	l := newLedger(64, room)
	frame := make([]byte, room/4)
	for i := uint64(1); i <= 8; i++ {
		l.begin(i)
		l.finish(i, frame)
	}
	if l.held > room {
		t.Fatalf("the table holds %d bytes for a bound of %d", l.held, room)
	}
	if got, err := l.begin(8); err != nil || len(got) != len(frame) {
		t.Fatalf("newest id: %d bytes err %v", len(got), err)
	}
	if _, err := l.begin(1); err == nil || !strings.Contains(err.Error(), "no longer held") {
		t.Fatalf("id evicted by bytes: %v", err)
	}
}

func TestLedgerDropReleasesAnID(t *testing.T) {
	l := newLedger(4, 1<<20)
	l.begin(11)
	l.drop(11)
	if frame, err := l.begin(11); err != nil || frame != nil {
		t.Fatalf("after drop: frame %v err %v", frame, err)
	}
}

func TestLedgerIsBoundedByCount(t *testing.T) {
	const max = 8
	l := newLedger(max, 1<<20)
	for i := uint64(1); i <= 3*max; i++ {
		l.begin(i)
		l.finish(i, []byte{byte(i)})
	}
	if len(l.done) > 2*max {
		t.Fatalf("the table holds %d ids for a bound of %d", len(l.done), max)
	}

	// The newest are answered from the record.
	frame, err := l.begin(3 * max)
	if err != nil || len(frame) != 1 {
		t.Fatalf("newest id: frame %v err %v", frame, err)
	}
	// One whose result was evicted is refused rather than run again.
	if _, err := l.begin(max + 4); err == nil || !strings.Contains(err.Error(), "no longer held") {
		t.Fatalf("evicted id: %v", err)
	}
	// Past the mark window an id is forgotten, and a repeat of it runs.
	if frame, err := l.begin(1); err != nil || frame != nil {
		t.Fatalf("forgotten id: frame %v err %v", frame, err)
	}
}
