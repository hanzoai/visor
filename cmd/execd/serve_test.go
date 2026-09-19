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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// host is the other end of the socketpair: what the sandbox's owner holds.
// The daemon runs in this process on the descriptor it was handed, exactly as
// it would on FD 3 under runsc.
type host struct {
	fd     int
	root   *root
	done   chan error
	t      *testing.T
	pushed []*rep // frames a handle pushed while call waited for a reply
}

func serveTree(t *testing.T) *host {
	t.Helper()
	r, _ := tree(t)

	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	mine, theirs := fds[0], fds[1]
	unix.SetsockoptInt(mine, unix.SOL_SOCKET, unix.SO_SNDBUF, 4*frameMax)
	unix.SetsockoptInt(theirs, unix.SOL_SOCKET, unix.SO_SNDBUF, 4*frameMax)
	if err := unix.SetsockoptTimeval(mine, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 20}); err != nil {
		t.Fatal(err)
	}

	h := &host{fd: mine, root: r, done: make(chan error, 1), t: t}
	d := newDaemon(theirs, r)
	go func() { h.done <- d.serve() }()
	t.Cleanup(func() {
		unix.Close(mine)
		if err := <-h.done; err != nil {
			t.Errorf("serve: %v", err)
		}
		unix.Close(theirs)
	})
	return h
}

func (h *host) send(r *req) {
	h.t.Helper()
	frame := r.encode()
	for {
		_, err := unix.Write(h.fd, frame)
		if err == unix.EINTR {
			continue // the runtime's own signals reach a blocking syscall
		}
		if err != nil {
			h.t.Fatalf("send: %v", err)
		}
		return
	}
}

func (h *host) recv() *rep {
	h.t.Helper()
	buf := make([]byte, frameMax)
	var n int
	for {
		got, _, err := unix.Recvfrom(h.fd, buf, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			h.t.Fatalf("recv: %v", err)
		}
		n = got
		break
	}
	r, err := decodeRep(buf[:n])
	if err != nil {
		h.t.Fatalf("decode: %v", err)
	}
	return r
}

// call sends one action and waits for the reply that names it. A frame pushed
// by a handle in the meantime is set aside, not dropped: replies and pushes
// interleave, and a test that asked for pty output still wants it.
func (h *host) call(r *req) *rep {
	h.t.Helper()
	h.send(r)
	for {
		got := h.recv()
		if got.kind == kindReply && got.id == r.id {
			return got
		}
		h.pushed = append(h.pushed, got)
	}
}

// next returns the next pushed frame, from what call set aside first.
func (h *host) next() *rep {
	h.t.Helper()
	if len(h.pushed) > 0 {
		got := h.pushed[0]
		h.pushed = h.pushed[1:]
		return got
	}
	return h.recv()
}

func (h *host) in(parts ...string) string {
	return filepath.Join(append([]string{h.root.name}, parts...)...)
}

func TestExecRunsAndReportsExit(t *testing.T) {
	h := serveTree(t)

	got := h.call(&req{op: opExec, id: 1, argv: []string{"/bin/sh", "-c", "echo out; echo err 1>&2; exit 3"}})
	if got.exit != 3 {
		t.Fatalf("exit %d err %q", got.exit, got.err)
	}
	if strings.TrimSpace(string(got.data)) != "out" || strings.TrimSpace(string(got.log)) != "err" {
		t.Fatalf("data %q log %q", got.data, got.log)
	}
}

func TestExecRunsACommandInADirectoryWithStdin(t *testing.T) {
	h := serveTree(t)

	got := h.call(&req{op: opExec, id: 1, command: "cat; pwd", dir: "src", data: []byte("fed in\n")})
	if got.exit != 0 || got.err != "" {
		t.Fatalf("exit %d err %q", got.exit, got.err)
	}
	want := "fed in\n" + h.in("src") + "\n"
	if string(got.data) != want {
		t.Fatalf("got %q want %q", got.data, want)
	}
}

func TestExecRefusesADirectoryOutsideTheWorkspace(t *testing.T) {
	h := serveTree(t)

	got := h.call(&req{op: opExec, id: 1, argv: []string{"/bin/true"}, dir: "../outside"})
	if got.err == "" {
		t.Fatal("a directory outside the workspace was accepted")
	}
}

func TestExecStopsAtItsTimeout(t *testing.T) {
	h := serveTree(t)

	got := h.call(&req{op: opExec, id: 1, argv: []string{"/bin/sh", "-c", "sleep 30"}, timeout: 200})
	if !strings.Contains(got.err, "no answer within") {
		t.Fatalf("err %q exit %d", got.err, got.exit)
	}
}

func TestExecRefusesArgvAndCommandTogether(t *testing.T) {
	h := serveTree(t)

	got := h.call(&req{op: opExec, id: 1, argv: []string{"/bin/true"}, command: "true"})
	if got.err == "" {
		t.Fatal("argv and command were both accepted")
	}
}

// A repeated id for a completed action answers the record. The effect has to
// happen once, which a side effect on disk proves.
func TestARepeatedIDDoesNotRunTheEffectTwice(t *testing.T) {
	h := serveTree(t)

	action := &req{op: opExec, id: 7821, command: "echo tick >> ticks"}
	first := h.call(action)
	if first.exit != 0 || first.err != "" {
		t.Fatalf("exit %d err %q", first.exit, first.err)
	}
	second := h.call(action)
	if second.exit != 0 || second.err != "" {
		t.Fatalf("replay: exit %d err %q", second.exit, second.err)
	}

	body, err := os.ReadFile(h.in("ticks"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "tick\n" {
		t.Fatalf("the command ran more than once: %q", body)
	}
}

func TestAZeroIDIsRefused(t *testing.T) {
	h := serveTree(t)

	h.send(&req{op: opExec, id: 0, argv: []string{"/bin/true"}})
	if got := h.recv(); got.err == "" {
		t.Fatal("an action with no id was carried out")
	}
}

func TestWriteThenReadAndStat(t *testing.T) {
	h := serveTree(t)

	body := []byte("package main\n\nfunc main() {}\n")
	if got := h.call(&req{op: opWrite, id: 1, path: "src/app.go", data: body}); got.err != "" {
		t.Fatalf("write: %s", got.err)
	}
	if got := h.call(&req{op: opRead, id: 2, path: "src/app.go"}); string(got.data) != string(body) {
		t.Fatalf("read %q err %q", got.data, got.err)
	}
	// A range, which is how a file over the read bound is fetched.
	got := h.call(&req{op: opRead, id: 3, path: "src/app.go", off: 8, length: 4})
	if string(got.data) != "main" {
		t.Fatalf("range read %q", got.data)
	}
	if got.size != uint64(len(body)) {
		t.Fatalf("size %d want %d", got.size, len(body))
	}

	st := h.call(&req{op: opStat, id: 4, path: "src"})
	if st.err != "" || st.flags&flagDir == 0 {
		t.Fatalf("stat src: %+v", st)
	}
}

func TestWriteIsAtomic(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("src", "main.go"), "old\n")
	link := h.in("src", "main.go")
	before, err := os.Stat(link)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.call(&req{op: opWrite, id: 1, path: "src/main.go", data: []byte("new\n")}); got.err != "" {
		t.Fatalf("write: %s", got.err)
	}
	after, err := os.Stat(link)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("the file was written in place, not renamed over")
	}
	body, _ := os.ReadFile(link)
	if string(body) != "new\n" {
		t.Fatalf("got %q", body)
	}
	if left, _ := filepath.Glob(h.in("src", ".*execd*")); len(left) != 0 {
		t.Fatalf("a temporary file was left behind: %v", left)
	}
}

func TestFileOpsRefuseAnEscape(t *testing.T) {
	h := serveTree(t)

	for i, r := range []*req{
		{op: opRead, path: "../outside/secret"},
		{op: opRead, path: "/etc/passwd"},
		{op: opRead, path: "away/secret"},
		{op: opWrite, path: "../outside/planted", data: []byte("x")},
		{op: opStat, path: "../outside/secret"},
		{op: opList, path: "../outside"},
	} {
		r.id = uint64(i + 1)
		if got := h.call(r); got.err == "" {
			t.Errorf("%s %s was allowed outside the workspace", opName(r.op), r.path)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(h.root.name), "outside", "planted")); err == nil {
		t.Fatal("a write landed outside the workspace")
	}
}

func TestListPagesADirectory(t *testing.T) {
	h := serveTree(t)

	for _, name := range []string{"a", "b", "c"} {
		write(t, h.in("src", name), name)
	}
	all := h.call(&req{op: opList, id: 1, path: "src"})
	if all.err != "" {
		t.Fatalf("list: %s", all.err)
	}
	if all.size != 4 || len(all.entries) != 4 { // a, b, c, main.go
		t.Fatalf("got %d entries of %d: %+v", len(all.entries), all.size, all.entries)
	}
	page := h.call(&req{op: opList, id: 2, path: "src", off: 2, length: 1})
	if len(page.entries) != 1 || page.entries[0].name != "c" {
		t.Fatalf("page %+v", page.entries)
	}
}

func TestPatchAppliesAtomically(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("src", "main.go"), "one\ntwo\nthree\n")
	diff := `--- a/src/main.go
+++ b/src/main.go
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
--- /dev/null
+++ b/src/extra.go
@@ -0,0 +1 @@
+made here
`
	got := h.call(&req{op: opPatch, id: 1, data: []byte(diff)})
	if got.err != "" {
		t.Fatalf("patch: %s", got.err)
	}
	if got.size != 2 || len(got.entries) != 2 {
		t.Fatalf("got %+v", got)
	}
	if body, _ := os.ReadFile(h.in("src", "main.go")); string(body) != "one\nTWO\nthree\n" {
		t.Fatalf("main.go is %q", body)
	}
	if body, _ := os.ReadFile(h.in("src", "extra.go")); string(body) != "made here\n" {
		t.Fatalf("extra.go is %q", body)
	}
}

func TestPatchChangesNothingWhenOneFileDoesNotFit(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("src", "main.go"), "one\ntwo\nthree\n")
	diff := `--- a/src/main.go
+++ b/src/main.go
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
--- a/src/absent.go
+++ b/src/absent.go
@@ -1,1 +1,1 @@
-was here
+is here
`
	got := h.call(&req{op: opPatch, id: 1, data: []byte(diff)})
	if got.err == "" {
		t.Fatal("a diff that does not fit was applied")
	}
	if body, _ := os.ReadFile(h.in("src", "main.go")); string(body) != "one\ntwo\nthree\n" {
		t.Fatalf("the first file changed anyway: %q", body)
	}
	if left, _ := filepath.Glob(h.in("src", ".*execd*")); len(left) != 0 {
		t.Fatalf("a temporary file was left behind: %v", left)
	}
}

func TestPatchRefusesAPathOutsideTheWorkspace(t *testing.T) {
	h := serveTree(t)

	diff := "--- a/../outside/secret\n+++ b/../outside/secret\n@@ -1,1 +1,1 @@\n-not yours\n+mine\n"
	if got := h.call(&req{op: opPatch, id: 1, data: []byte(diff)}); got.err == "" {
		t.Fatal("a diff reached outside the workspace")
	}
	if body, _ := os.ReadFile(filepath.Join(filepath.Dir(h.root.name), "outside", "secret")); string(body) != "not yours\n" {
		t.Fatalf("the file outside changed: %q", body)
	}
}

func TestGitRunsInTheWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	h := serveTree(t)

	if got := h.call(&req{op: opGit, id: 1, argv: []string{"init", "-q"}}); got.exit != 0 {
		t.Fatalf("init: exit %d err %q %s", got.exit, got.err, got.log)
	}
	got := h.call(&req{op: opGit, id: 2, argv: []string{"rev-parse", "--show-toplevel"}})
	if got.exit != 0 {
		t.Fatalf("rev-parse: exit %d %s", got.exit, got.log)
	}
	if strings.TrimSpace(string(got.data)) != h.root.name {
		t.Fatalf("git ran in %q, not %q", strings.TrimSpace(string(got.data)), h.root.name)
	}
	if got := h.call(&req{op: opGit, id: 3, command: "status"}); got.err == "" {
		t.Fatal("git took a command string")
	}
}

func TestSpawnDrivesAPty(t *testing.T) {
	h := serveTree(t)

	start := h.call(&req{op: opSpawn, id: 1, argv: []string{"/bin/sh", "-i"}, cols: 100, rows: 30})
	if start.err != "" || start.handle == 0 {
		t.Fatalf("spawn: %+v", start)
	}
	n := start.handle

	if got := h.call(&req{op: opResize, id: 2, handle: n, cols: 120, rows: 40}); got.err != "" {
		t.Fatalf("resize: %s", got.err)
	}
	if got := h.call(&req{op: opPtyWrite, id: 3, handle: n, data: []byte("echo it-runs\nexit\n")}); got.err != "" {
		t.Fatalf("pty.write: %s", got.err)
	}

	var out strings.Builder
	for {
		got := h.next()
		if got.kind == kindData {
			out.Write(got.data)
			continue
		}
		if got.kind == kindExit {
			if got.handle != n || got.id != 1 {
				t.Fatalf("exit names handle %d action %d", got.handle, got.id)
			}
			break
		}
	}
	if !strings.Contains(out.String(), "it-runs") {
		t.Fatalf("the pty said %q", out.String())
	}
}

func TestKillEndsAPty(t *testing.T) {
	h := serveTree(t)

	start := h.call(&req{op: opSpawn, id: 1, argv: []string{"/bin/sh", "-c", "sleep 60"}})
	if start.err != "" {
		t.Fatalf("spawn: %s", start.err)
	}
	if got := h.call(&req{op: opKill, id: 2, handle: start.handle, signal: int32(unix.SIGKILL)}); got.err != "" {
		t.Fatalf("kill: %s", got.err)
	}
	for {
		if got := h.next(); got.kind == kindExit {
			if !strings.Contains(got.err, "killed by") {
				t.Fatalf("exit says %q", got.err)
			}
			break
		}
	}
	if got := h.call(&req{op: opPtyWrite, id: 3, handle: start.handle, data: []byte("x")}); got.err == "" {
		t.Fatal("a closed handle still took a write")
	}
}

func TestWatchReportsAChange(t *testing.T) {
	h := serveTree(t)

	start := h.call(&req{op: opWatch, id: 1, path: "src"})
	if start.err != "" || start.handle == 0 {
		t.Fatalf("watch: %+v", start)
	}
	h.send(&req{op: opWrite, id: 2, path: "src/fresh.go", data: []byte("hi\n")})

	var seen bool
	for !seen {
		got := h.next()
		if got.kind == kindEvent && got.id == 1 && got.path == "fresh.go" && got.mask != 0 {
			seen = true
		}
	}
	if got := h.call(&req{op: opKill, id: 3, handle: start.handle}); got.err != "" {
		t.Fatalf("kill a watch: %s", got.err)
	}
	if got := h.call(&req{op: opKill, id: 4, handle: start.handle}); got.err == "" {
		t.Fatal("a closed watch handle was found again")
	}
}

func TestAnUnknownOpIsRefused(t *testing.T) {
	h := serveTree(t)

	if got := h.call(&req{op: 200, id: 1}); !strings.Contains(got.err, "unknown op") {
		t.Fatalf("got %q", got.err)
	}
}

func TestAReadOverTheBoundIsRefusedWithItsSize(t *testing.T) {
	h := serveTree(t)

	big := make([]byte, readMax+1)
	write(t, h.in("big"), string(big))
	got := h.call(&req{op: opRead, id: 1, path: "big"})
	if !strings.Contains(got.err, "over the") {
		t.Fatalf("got %q", got.err)
	}
	if got := h.call(&req{op: opRead, id: 2, path: "big", length: 8}); len(got.data) != 8 {
		t.Fatalf("a range of a big file read %d bytes: %q", len(got.data), got.err)
	}
}

// A repository above the workspace is not the workspace's repository. git's
// ceiling has to name the workspace's parent for that to hold from the
// workspace root, which is where a request that names no directory runs.
func TestGitCannotReachARepositoryAboveTheWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	h := serveTree(t)
	above := filepath.Dir(h.root.name)
	outer := func(argv ...string) string {
		t.Helper()
		cmd := exec.Command("git", argv...)
		cmd.Dir = above
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v above the workspace: %v %s", argv, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	sign := []string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}
	outer("init", "-q", "-b", "main")
	outer(append(append([]string{}, sign...), "commit", "-q", "--allow-empty", "-m", "outer")...)
	before := outer("rev-list", "--count", "HEAD")

	for i, argv := range [][]string{
		{"rev-parse", "--show-toplevel"},
		{"log", "--oneline"},
		append(append([]string{}, sign...), "commit", "--allow-empty", "-m", "written-from-inside"),
		{"config", "--local", "execd.planted", "yes"},
	} {
		if got := h.call(&req{op: opGit, id: uint64(i + 1), argv: argv}); got.exit == 0 {
			t.Errorf("git %v reached the repository above the workspace: %q", argv, got.data)
		}
	}
	if got := h.call(&req{op: opGit, id: 10, argv: []string{"rev-parse", "--show-toplevel"}, dir: "src"}); got.exit == 0 {
		t.Errorf("git in a subdirectory reached %q", got.data)
	}
	if after := outer("rev-list", "--count", "HEAD"); after != before {
		t.Fatalf("the repository above the workspace went from %s commits to %s", before, after)
	}
	if out := outer("config", "--local", "--list"); strings.Contains(out, "execd.planted") {
		t.Fatalf("its config was written: %q", out)
	}
}

// A zero-length packet is a packet a sequenced-packet socket delivers. It asks
// for nothing, and it is not the host hanging up.
func TestAnEmptyPacketIsNotEndOfFile(t *testing.T) {
	h := serveTree(t)

	if got := h.call(&req{op: opWrite, id: 1, path: "before", data: []byte("x\n")}); got.err != "" {
		t.Fatalf("write: %s", got.err)
	}
	for {
		_, err := unix.Write(h.fd, nil)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			t.Fatalf("send an empty packet: %v", err)
		}
		break
	}
	if got := h.call(&req{op: opWrite, id: 2, path: "after", data: []byte("y\n")}); got.err != "" {
		t.Fatalf("after an empty packet: %s", got.err)
	}
}

// The default page is one a frame can hold, and off walks a directory no
// single frame could ever carry.
func TestListPagesADirectoryTooBigForAFrame(t *testing.T) {
	h := serveTree(t)

	const total = 4096
	if err := os.Mkdir(h.in("many"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < total; i++ {
		write(t, h.in("many", fmt.Sprintf("file-%06d.txt", i)), "")
	}

	seen := 0
	for id := uint64(1); seen < total; id++ {
		got := h.call(&req{op: opList, id: id, path: "many", off: int64(seen)})
		if got.err != "" {
			t.Fatalf("list from %d: %s", seen, got.err)
		}
		if got.size != total {
			t.Fatalf("size %d, want %d", got.size, total)
		}
		if len(got.entries) == 0 {
			t.Fatalf("an empty page at %d of %d", seen, total)
		}
		for i, e := range got.entries {
			if want := fmt.Sprintf("file-%06d.txt", seen+i); e.name != want {
				t.Fatalf("entry %d is %q, want %q", seen+i, e.name, want)
			}
		}
		seen += len(got.entries)
	}
}

// One path string means one place. away is a symlink out of the workspace, so
// every op refuses a path through it — a write is not more permissive than a
// read because it looked at the string instead of asking the kernel.
func TestEveryOpAgreesOnAnEscapingComponent(t *testing.T) {
	h := serveTree(t)

	const p = "away/../redirected"
	if got := h.call(&req{op: opRead, id: 1, path: p}); got.err == "" {
		t.Error("a read through a symlink out of the workspace was allowed")
	}
	if got := h.call(&req{op: opWrite, id: 2, path: p, data: []byte("x\n")}); got.err == "" {
		t.Error("a write through a symlink out of the workspace was allowed")
	}
	diff := "--- /dev/null\n+++ b/away/../redirected\n@@ -0,0 +1 @@\n+x\n"
	if got := h.call(&req{op: opPatch, id: 3, data: []byte(diff)}); got.err == "" {
		t.Error("a patch through a symlink out of the workspace was allowed")
	}
	for _, name := range []string{h.in("redirected"), filepath.Join(filepath.Dir(h.root.name), "redirected"), filepath.Join(filepath.Dir(h.root.name), "outside", "redirected")} {
		if _, err := os.Stat(name); err == nil {
			t.Errorf("something landed at %s", name)
		}
	}
}

// A ".." that stays beneath the workspace is a path like any other.
func TestADotDotThatStaysInsideIsTakenAsWritten(t *testing.T) {
	h := serveTree(t)

	got := h.call(&req{op: opRead, id: 1, path: "src/../src/main.go"})
	if got.err != "" || string(got.data) != "package main\n" {
		t.Fatalf("read: %q err %q", got.data, got.err)
	}
	if got := h.call(&req{op: opWrite, id: 2, path: "src/../src/fresh.go", data: []byte("hi\n")}); got.err != "" {
		t.Fatalf("write: %s", got.err)
	}
	if body, _ := os.ReadFile(h.in("src", "fresh.go")); string(body) != "hi\n" {
		t.Fatalf("got %q", body)
	}
}

// A refused request did not happen, so its id is free and asking again runs.
func TestARefusedRequestLeavesItsIDFree(t *testing.T) {
	h := serveTree(t)

	if got := h.call(&req{op: opWrite, id: 99, path: "../outside/planted", data: []byte("x")}); got.err == "" {
		t.Fatal("a write outside the workspace was accepted")
	}
	if got := h.call(&req{op: opWrite, id: 99, path: "kept", data: []byte("x\n")}); got.err != "" {
		t.Fatalf("a refusal froze the id: %s", got.err)
	}
	if body, _ := os.ReadFile(h.in("kept")); string(body) != "x\n" {
		t.Fatalf("got %q", body)
	}
}

// An action that ran is recorded whatever came of it. A command killed by its
// deadline ran, so a repeat of the id answers the record: a second deadline is
// a second action and takes an id of its own.
func TestATimeoutIsRecordedBecauseItRan(t *testing.T) {
	h := serveTree(t)

	action := &req{op: opExec, id: 8001, command: "echo ran >> tries; sleep 30", timeout: 200}
	first := h.call(action)
	if !strings.Contains(first.err, "no answer within") {
		t.Fatalf("err %q exit %d", first.err, first.exit)
	}
	again := &req{op: opExec, id: 8001, command: "echo ran >> tries; sleep 30"}
	if second := h.call(again); second.err != first.err {
		t.Fatalf("the record says %q, want %q", second.err, first.err)
	}
	body, err := os.ReadFile(h.in("tries"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ran\n" {
		t.Fatalf("the command ran again: %q", body)
	}
}

// A staged file's name is not one a request can name, so a file of the
// workspace's own is never in the way of a write.
func TestWriteKeepsAFileThatLooksStaged(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("f"), "one\n")
	write(t, h.in(".f.execd5300"), "mine\n")
	write(t, h.in(".execd5300"), "mine too\n")
	if got := h.call(&req{op: opWrite, id: 5300, path: "f", data: []byte("two\n")}); got.err != "" {
		t.Fatalf("write: %s", got.err)
	}
	for _, name := range []string{".f.execd5300", ".execd5300"} {
		if body, err := os.ReadFile(h.in(name)); err != nil || !strings.HasPrefix(string(body), "mine") {
			t.Errorf("%s: %q %v", name, body, err)
		}
	}
	if left, _ := filepath.Glob(h.in(".execd*")); len(left) != 1 {
		t.Fatalf("staged files left behind: %v", left)
	}
}

func TestWriteReportsTheModeOnDisk(t *testing.T) {
	h := serveTree(t)
	was := unix.Umask(0o022)
	t.Cleanup(func() { unix.Umask(was) })

	got := h.call(&req{op: opWrite, id: 1, path: "runme", data: []byte("#!/bin/sh\n"), mode: 0o777})
	if got.err != "" {
		t.Fatalf("write: %s", got.err)
	}
	st, err := os.Stat(h.in("runme"))
	if err != nil {
		t.Fatal(err)
	}
	if got.mode != uint32(st.Mode().Perm()) {
		t.Fatalf("the reply says %#o and the file is %#o", got.mode, st.Mode().Perm())
	}
	if st.Mode().Perm() != 0o777 {
		t.Fatalf("the mode asked for was not the mode set: %#o", st.Mode().Perm())
	}
}

func TestWriteKeepsTheModeAFileHad(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("script"), "old\n")
	if err := os.Chmod(h.in("script"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := h.call(&req{op: opWrite, id: 1, path: "script", data: []byte("new\n")})
	if got.err != "" || got.mode != 0o755 {
		t.Fatalf("write says mode %#o err %q", got.mode, got.err)
	}
	st, err := os.Stat(h.in("script"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("the file is %#o", st.Mode().Perm())
	}
}

// One field, one meaning: mode is permission bits, wherever it is answered.
func TestStatListAndReadAgreeOnMode(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("src", "plain.txt"), "x\n")
	if err := os.Chmod(h.in("src", "plain.txt"), 0o640); err != nil {
		t.Fatal(err)
	}
	st := h.call(&req{op: opStat, id: 1, path: "src/plain.txt"})
	rd := h.call(&req{op: opRead, id: 2, path: "src/plain.txt"})
	ls := h.call(&req{op: opList, id: 3, path: "src"})
	var listed uint32
	for _, e := range ls.entries {
		if e.name == "plain.txt" {
			listed = e.mode
		}
	}
	if st.mode != 0o640 || rd.mode != 0o640 || listed != 0o640 {
		t.Fatalf("stat %#o read %#o list %#o", st.mode, rd.mode, listed)
	}
}

func TestReadPastTheEndIsRefused(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("small"), "x\n")
	if got := h.call(&req{op: opRead, id: 1, path: "small", off: 1 << 40}); got.err == "" {
		t.Fatalf("a read a terabyte past the end answered %d bytes of a %d byte file", len(got.data), got.size)
	}
	if got := h.call(&req{op: opRead, id: 2, path: "small", off: 2}); got.err != "" || len(got.data) != 0 {
		t.Fatalf("a read at the end: %q err %q", got.data, got.err)
	}
}

func TestAnOversizeRequestNamesTheOpAndTheAction(t *testing.T) {
	h := serveTree(t)

	frame := (&req{op: opWrite, id: 5600, path: "toobig", data: make([]byte, frameMax)}).encode()
	if len(frame) <= frameMax {
		t.Fatalf("the frame is %d bytes, which fits", len(frame))
	}
	for {
		_, err := unix.Write(h.fd, frame)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		break
	}
	got := h.recv()
	if got.id != 5600 || got.op != opWrite {
		t.Fatalf("the refusal names op %d action %d", got.op, got.id)
	}
	if !strings.Contains(got.err, "over the") {
		t.Fatalf("err %q", got.err)
	}
	if _, err := os.Stat(h.in("toobig")); err == nil {
		t.Fatal("the oversize write landed")
	}
}

// execd holds no credential, and a child starts from a declared environment,
// so what the host handed the daemon does not reach a command.
func TestAChildGetsADeclaredEnvironment(t *testing.T) {
	h := serveTree(t)
	t.Setenv("EXECD_TEST_SECRET", "sk-do-not-leak")

	got := h.call(&req{op: opExec, id: 1, command: "echo [$EXECD_TEST_SECRET]; echo $PATH"})
	if got.err != "" {
		t.Fatalf("exec: %s", got.err)
	}
	if strings.Contains(string(got.data), "sk-do-not-leak") {
		t.Fatalf("the daemon's environment reached the child: %q", got.data)
	}
	if !strings.Contains(string(got.data), "/usr/bin") {
		t.Fatalf("the child got no PATH: %q", got.data)
	}

	got = h.call(&req{op: opExec, id: 2, argv: []string{"/bin/sh", "-c", "echo $ONLY; echo [$EXECD_TEST_SECRET]"},
		env: []string{"ONLY=this"}})
	if want := "this\n[]\n"; string(got.data) != want {
		t.Fatalf("got %q want %q", got.data, want)
	}
}

func TestPatchNamingOneFileTwiceChangesNothing(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("f"), "one\n")
	diff := "--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-one\n+first\n" +
		"--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-one\n+second\n"
	if got := h.call(&req{op: opPatch, id: 1, data: []byte(diff)}); got.err == "" {
		t.Fatal("a diff naming one file twice was applied")
	}
	if body, _ := os.ReadFile(h.in("f")); string(body) != "one\n" {
		t.Fatalf("f is %q", body)
	}
	if left, _ := filepath.Glob(h.in(".execd*")); len(left) != 0 {
		t.Fatalf("staged files left behind: %v", left)
	}
}

// Two names for one file are one file, whatever the strings look like.
func TestPatchRefusesTwoNamesForOneFile(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("f"), "one\n")
	if err := os.Symlink("f", h.in("g")); err != nil {
		t.Fatal(err)
	}
	diff := "--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-one\n+first\n" +
		"--- a/g\n+++ b/g\n@@ -1,1 +1,1 @@\n-one\n+second\n"
	got := h.call(&req{op: opPatch, id: 1, data: []byte(diff)})
	if got.err == "" || !strings.Contains(got.err, "one file") {
		t.Fatalf("err %q", got.err)
	}
	if body, _ := os.ReadFile(h.in("f")); string(body) != "one\n" {
		t.Fatalf("f is %q", body)
	}
}

func TestPatchThatWouldDeleteADirectoryChangesNothing(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("a.txt"), "one\n")
	diff := "--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-one\n+ONE\n" +
		"--- a/src\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-nothing\n"
	if got := h.call(&req{op: opPatch, id: 1, data: []byte(diff)}); got.err == "" {
		t.Fatal("a diff deleted a directory")
	}
	if body, _ := os.ReadFile(h.in("a.txt")); string(body) != "one\n" {
		t.Fatalf("the first file changed anyway: %q", body)
	}
	if _, err := os.Stat(h.in("src")); err != nil {
		t.Fatalf("src: %v", err)
	}
	if left, _ := filepath.Glob(h.in(".execd*")); len(left) != 0 {
		t.Fatalf("staged files left behind: %v", left)
	}
}

func TestPatchDeletesAFile(t *testing.T) {
	h := serveTree(t)

	write(t, h.in("gone.txt"), "bye\n")
	diff := "--- a/gone.txt\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-bye\n"
	if got := h.call(&req{op: opPatch, id: 1, data: []byte(diff)}); got.err != "" {
		t.Fatalf("patch: %s", got.err)
	}
	if _, err := os.Stat(h.in("gone.txt")); !os.IsNotExist(err) {
		t.Fatalf("the file is still there: %v", err)
	}
	if left, _ := filepath.Glob(h.in(".execd*")); len(left) != 0 {
		t.Fatalf("staged files left behind: %v", left)
	}
	if got := h.call(&req{op: opPatch, id: 2, data: []byte(diff)}); got.err == "" {
		t.Fatal("a diff deleted a file that was not there")
	}
}
