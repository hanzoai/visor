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
	"sync"
)

// ledger remembers the reply of every completed action, keyed by its id, so
// a request the host repeats after losing an answer is answered from the
// record instead of run a second time.
//
// It is bounded by count: the last max replies are held, and the max ids
// evicted before those are kept as bare marks — enough to refuse a repeat
// whose result is gone rather than run the effect twice.
type ledger struct {
	mu   sync.Mutex
	max  int
	done map[uint64][]byte // nil value: completed, result no longer held
	kept []uint64          // ids whose result is held, oldest first
	mark []uint64          // ids kept as marks, oldest first
	live map[uint64]bool
}

func newLedger(max int) *ledger {
	if max < 1 {
		max = 1
	}
	return &ledger{
		max:  max,
		done: make(map[uint64][]byte, 2*max),
		live: make(map[uint64]bool),
	}
}

// begin claims an id. It returns the recorded reply when the action already
// completed, and an error when the id is running, or completed with its
// result no longer held.
func (l *ledger) begin(id uint64) ([]byte, error) {
	if id == 0 {
		return nil, fmt.Errorf("action id is zero")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if frame, ok := l.done[id]; ok {
		if frame == nil {
			return nil, fmt.Errorf("action %d completed and its result is no longer held", id)
		}
		return frame, nil
	}
	if l.live[id] {
		return nil, fmt.Errorf("action %d is already running", id)
	}
	l.live[id] = true
	return nil, nil
}

// finish records an action's reply under its id.
func (l *ledger) finish(id uint64, frame []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.live, id)
	if _, ok := l.done[id]; ok {
		return
	}
	l.done[id] = frame
	l.kept = append(l.kept, id)
	for len(l.kept) > l.max {
		old := l.kept[0]
		l.kept = l.kept[1:]
		l.done[old] = nil
		l.mark = append(l.mark, old)
	}
	for len(l.mark) > l.max {
		old := l.mark[0]
		l.mark = l.mark[1:]
		delete(l.done, old)
	}
}

// drop releases an id that never completed, so the host may retry it.
func (l *ledger) drop(id uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.live, id)
}
