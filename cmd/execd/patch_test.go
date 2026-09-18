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
