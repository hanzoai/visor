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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyEditsALine(t *testing.T) {
	old := "one\ntwo\nthree\n"
	diff := `--- a/f
+++ b/f
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
`
	got := applyDiff(t, diff, old)
	if want := "one\nTWO\nthree\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyCreatesAFile(t *testing.T) {
	diff := `--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+alpha
+beta
`
	changes, err := parseDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].path != "new.txt" || changes[0].kill {
		t.Fatalf("got %+v", changes)
	}
	out, err := apply("new.txt", nil, changes[0].hunks)
	if err != nil {
		t.Fatal(err)
	}
	if want := "alpha\nbeta\n"; string(out) != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestParseReadsADeletion(t *testing.T) {
	diff := `--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
`
	changes, err := parseDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].path != "gone.txt" || !changes[0].kill {
		t.Fatalf("got %+v", changes)
	}
}

func TestApplyKeepsAMissingFinalNewline(t *testing.T) {
	diff := `--- a/f
+++ b/f
@@ -1,2 +1,2 @@
 keep
-old
\ No newline at end of file
+new
\ No newline at end of file
`
	got := applyDiff(t, diff, "keep\nold")
	if want := "keep\nnew"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyRefusesAContextThatDoesNotMatch(t *testing.T) {
	diff := `--- a/f
+++ b/f
@@ -1,2 +1,2 @@
 one
-two
+TWO
`
	changes, err := parseDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	_, err = apply("f", []byte("one\nSOMETHING ELSE\n"), changes[0].hunks)
	if err == nil {
		t.Fatal("a hunk applied against the wrong content")
	}
	if !strings.Contains(err.Error(), "expects") {
		t.Fatalf("the refusal does not say what it wanted: %v", err)
	}
}

func TestApplyTakesTwoHunks(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\n"
	diff := `--- a/f
+++ b/f
@@ -1,3 +1,3 @@
 a
-b
+B
 c
@@ -5,3 +5,3 @@
 e
-f
+F
 g
`
	got := applyDiff(t, diff, old)
	if want := "a\nB\nc\nd\ne\nF\ng\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseRefusesNonsense(t *testing.T) {
	for _, diff := range []string{
		"@@ -1,1 +1,1 @@\n one\n",                  // a hunk before any file
		"--- a/f\n",                                // --- without +++
		"--- a/f\n+++ b/f\n@@ nonsense @@\n",       // an unreadable header
		"--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\nfoo\n", // a line that is not a hunk line
	} {
		if _, err := parseDiff(diff); err == nil {
			t.Errorf("parsed %q", diff)
		}
	}
}

func applyDiff(t *testing.T, diff, old string) string {
	t.Helper()
	changes, err := parseDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("the diff names %d files", len(changes))
	}
	out, err := apply(changes[0].path, []byte(old), changes[0].hunks)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// A diff written without git's a/ and b/ prefixes names the file it names,
// even when the first component happens to be "a".
func TestParseReadsADiffWrittenWithoutPrefixes(t *testing.T) {
	for _, c := range []struct {
		diff, path string
	}{
		{"--- f\n+++ f\n@@ -1,1 +1,1 @@\n-one\n+ONE\n", "f"},
		{"--- a/f\n+++ a/f\n@@ -1,1 +1,1 @@\n-one\n+ONE\n", "a/f"},
		{"--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-one\n+ONE\n", "f"},
		{"--- src/a/f\n+++ src/a/f\n@@ -1,1 +1,1 @@\n-one\n+ONE\n", "src/a/f"},
	} {
		changes, err := parseDiff(c.diff)
		if err != nil {
			t.Fatalf("%q: %v", c.diff, err)
		}
		if len(changes) != 1 || changes[0].path != c.path {
			t.Errorf("%q names %+v, want %s", c.diff, changes, c.path)
		}
	}
}

func TestParseRefusesTwoSidesThatDisagree(t *testing.T) {
	diff := "--- a/one\n+++ b/two\n@@ -1,1 +1,1 @@\n-x\n+y\n"
	if _, err := parseDiff(diff); err == nil || !strings.Contains(err.Error(), "on the other") {
		t.Fatalf("got %v", err)
	}
}

func TestParseRefusesOneFileTwice(t *testing.T) {
	for _, diff := range []string{
		"--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-one\n+ONE\n--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-ONE\n+one\n",
		"--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-one\n+ONE\n--- a/./f\n+++ b/./f\n@@ -1,1 +1,1 @@\n-ONE\n+one\n",
	} {
		if _, err := parseDiff(diff); err == nil || !strings.Contains(err.Error(), "already changes") {
			t.Errorf("%q: %v", diff, err)
		}
	}
}

func TestParseRefusesARename(t *testing.T) {
	diff := "diff --git a/one b/two\nsimilarity index 100%\nrename from one\nrename to two\n"
	if _, err := parseDiff(diff); err == nil || !strings.Contains(err.Error(), "moves a file") {
		t.Fatalf("got %v", err)
	}
}

// A move that fails puts back the moves before it, so the tree is the tree the
// diff was applied to. The second move names a staged file that is not there,
// which is what losing a race with something else in the workspace looks like.
func TestCommitUndoesWhatItMoved(t *testing.T) {
	r, _ := tree(t)
	d := newDaemon(-1, r)
	write(t, filepath.Join(r.name, "one"), "ONE\n")
	write(t, filepath.Join(r.name, "two"), "TWO\n")

	changes, err := parseDiff("--- a/one\n+++ b/one\n@@ -1,1 +1,1 @@\n-ONE\n+1\n" +
		"--- a/two\n+++ b/two\n@@ -1,1 +1,1 @@\n-TWO\n+2\n")
	if err != nil {
		t.Fatal(err)
	}
	work, err := d.plan(changes)
	if err != nil {
		t.Fatal(err)
	}
	defer release(work)

	staged := work[1].next
	work[1].next = ".execd-is-not-there"
	untouched, err := commit(work)
	if err == nil {
		t.Fatal("a move from a staged file that is not there succeeded")
	}
	if !untouched {
		t.Fatalf("the tree was left part way through: %v", err)
	}
	for _, c := range []struct{ name, body string }{{"one", "ONE\n"}, {"two", "TWO\n"}} {
		if body, _ := os.ReadFile(filepath.Join(r.name, c.name)); string(body) != c.body {
			t.Errorf("%s is %q, want %q", c.name, body, c.body)
		}
	}
	os.Remove(filepath.Join(r.name, staged))
}
