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
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// null is the path a unified diff gives the missing side of a created or
// deleted file.
const null = "/dev/null"

// change is one file's worth of a unified diff.
type change struct {
	path  string
	kill  bool // the diff deletes the file
	hunks []hunk
}

// hunk is one @@ block. Its lines keep their leading ' ', '-' or '+'.
type hunk struct {
	old, new int // 1-based first line on each side
	lines    []string
	noEndNew bool // the new side ends without a newline
}

// move is one file's worth of a staged diff: what goes in, what was there,
// and the directory both of them live in.
type move struct {
	dirfd int
	base  string
	path  string
	next  string // the staged replacement; empty when the diff deletes the file
	prev  string // a link to the bytes that were there; empty when there were none
}

// patch applies a unified diff. Every file is verified and staged first, and
// the bytes that were there are kept on a second link, so a diff that does
// not fit changes nothing and a move that fails is put back.
func (d *daemon) patch(r *req) *rep {
	changes, err := parseDiff(string(r.data))
	if err != nil {
		return fail(r, err)
	}
	if len(changes) == 0 {
		return fail(r, errors.New("the diff names no file"))
	}

	work, err := d.plan(changes)
	if err != nil {
		return fail(r, err)
	}
	defer release(work)

	if untouched, err := commit(work); err != nil {
		reply := fail(r, err)
		// A diff that was put back did not happen; one the daemon could not
		// put back did, and says so, so a repeat of the id is not run again.
		reply.refused = untouched
		return reply
	}

	reply := &rep{size: uint64(len(work))}
	for _, m := range work {
		reply.entries = append(reply.entries, entry{name: m.path})
	}
	return reply
}

// plan verifies and stages every change. Nothing in the workspace is moved
// yet: on the way out with an error, every staged file is gone.
func (d *daemon) plan(changes []change) ([]move, error) {
	var work []move
	seen := make(map[[2]uint64]string, len(changes))
	for _, c := range changes {
		m, err := d.arrange(c, seen)
		if err != nil {
			release(work)
			return nil, err
		}
		work = append(work, m)
	}
	return work, nil
}

// arrange stages one change: the new bytes in a file of their own, and a link
// to the old bytes to go back to. seen holds the files the diff has already
// named, by identity, so two spellings of one file are refused rather than
// applied one over the other.
func (d *daemon) arrange(c change, seen map[[2]uint64]string) (move, error) {
	dirfd, base, err := d.root.parent(c.path)
	if err != nil {
		return move{}, err
	}
	m := move{dirfd: dirfd, base: base, path: c.path}
	kept := false
	defer func() {
		if !kept {
			release([]move{m})
		}
	}()

	var st unix.Stat_t
	here := unix.Fstatat(dirfd, base, &st, 0) == nil
	switch {
	case here && st.Mode&unix.S_IFMT == unix.S_IFDIR:
		return move{}, fmt.Errorf("%s: is a directory", c.path)
	case here:
		if first, ok := seen[[2]uint64{st.Dev, st.Ino}]; ok {
			return move{}, fmt.Errorf("the diff names %s and %s, which are one file", first, c.path)
		}
		seen[[2]uint64{st.Dev, st.Ino}] = c.path
	case c.kill:
		return move{}, fmt.Errorf("delete %s: %w", c.path, unix.ENOENT)
	}

	if !c.kill {
		old, mode, err := d.current(c.path)
		if err != nil {
			return move{}, err
		}
		next, err := apply(c.path, old, c.hunks)
		if err != nil {
			return move{}, err
		}
		name, fd, err := d.stage(dirfd, mode)
		if err != nil {
			return move{}, err
		}
		m.next = name
		if _, err := settle(os.NewFile(uintptr(fd), name), next, mode); err != nil {
			return move{}, err
		}
	}
	if here {
		prev, err := link(dirfd, base)
		if err != nil {
			return move{}, fmt.Errorf("keep %s to go back to: %w", c.path, err)
		}
		m.prev = prev
	}
	kept = true
	return m, nil
}

// link hard-links the file at base beside it under a name nothing else can
// name. The link holds the old bytes whatever happens to base.
func link(dirfd int, base string) (string, error) {
	for try := 0; try < 4; try++ {
		name, err := scratch()
		if err != nil {
			return "", err
		}
		err = unix.Linkat(dirfd, base, dirfd, name, 0)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, unix.EEXIST) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free name beside %s", base)
}

// commit moves every staged file into place. A move that fails puts back the
// moves before it; the bool says whether the tree is as it was, which is what
// decides whether the action can be asked for again.
func commit(work []move) (bool, error) {
	var err error
	for i, m := range work {
		if m.next == "" {
			err = unix.Unlinkat(m.dirfd, m.base, 0)
			if err != nil {
				err = fmt.Errorf("delete %s: %w", m.path, err)
			}
		} else if err = unix.Renameat(m.dirfd, m.next, m.dirfd, m.base); err != nil {
			err = fmt.Errorf("rename over %s: %w", m.path, err)
		}
		if err == nil {
			continue
		}
		if back := undo(work[:i]); back != nil {
			return false, fmt.Errorf("%w; and %d of %d files are changed: %w", err, i, len(work), back)
		}
		return true, err
	}
	return false, nil
}

// undo puts back what commit has moved, newest first: a file that was there is
// restored from its link, and one the diff created is removed.
func undo(work []move) error {
	for i := len(work) - 1; i >= 0; i-- {
		m := work[i]
		if m.prev != "" {
			if err := unix.Renameat(m.dirfd, m.prev, m.dirfd, m.base); err != nil {
				return fmt.Errorf("put %s back: %w", m.path, err)
			}
			continue
		}
		if err := unix.Unlinkat(m.dirfd, m.base, 0); err != nil {
			return fmt.Errorf("take %s away again: %w", m.path, err)
		}
	}
	return nil
}

// release lets go of staged work. Before commit it takes the staged files and
// the links away; after commit the staged file is the file and the link is the
// last hold on the bytes it replaced, so the same call is what finishes a
// delete and frees what a write replaced.
func release(work []move) {
	for _, m := range work {
		if m.next != "" {
			unix.Unlinkat(m.dirfd, m.next, 0)
		}
		if m.prev != "" {
			unix.Unlinkat(m.dirfd, m.prev, 0)
		}
		unix.Close(m.dirfd)
	}
}

// current reads the file a change applies to. A file the diff creates is
// absent, and that is not an error.
func (d *daemon) current(name string) ([]byte, uint32, error) {
	f, err := d.root.file(name, unix.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, fileMode, nil
		}
		return nil, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, err
	}
	return data, uint32(st.Mode().Perm()), nil
}

// parseDiff reads a unified diff. Anything it does not understand is an
// error: a patch is never applied on a guess.
func parseDiff(text string) ([]change, error) {
	lines := strings.Split(text, "\n")
	var out []change
	var cur *change
	var h *hunk
	named := make(map[string]string)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "--- "):
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
				return nil, fmt.Errorf("line %d: --- without +++", i+1)
			}
			name, kill, err := sides(line[4:], lines[i+1][4:])
			i++
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			if first, ok := named[path.Clean(name)]; ok {
				return nil, fmt.Errorf("line %d: the diff already changes %s", i+1, first)
			}
			named[path.Clean(name)] = name
			out = append(out, change{path: name, kill: kill})
			cur, h = &out[len(out)-1], nil

		case strings.HasPrefix(line, "@@"):
			if cur == nil {
				return nil, fmt.Errorf("line %d: a hunk before any file", i+1)
			}
			head, err := parseHunk(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			cur.hunks = append(cur.hunks, head)
			h = &cur.hunks[len(cur.hunks)-1]

		case h != nil && line == `\ No newline at end of file`:
			if n := len(h.lines); n > 0 && h.lines[n-1][0] != '-' {
				h.noEndNew = true
			}

		case h != nil && line != "" && (line[0] == ' ' || line[0] == '+' || line[0] == '-'):
			h.lines = append(h.lines, line)

		case line == "":
			// A context line is written as a single space, so an empty line
			// inside a hunk is a diff this daemon will not guess at.
			if h != nil && i != len(lines)-1 {
				return nil, fmt.Errorf("line %d: an empty line inside a hunk", i+1)
			}

		case strings.HasPrefix(line, "rename ") || strings.HasPrefix(line, "copy "):
			return nil, fmt.Errorf("line %d: %q moves a file, which a diff of hunks cannot say; send the delete and the create", i+1, strings.TrimSpace(line))

		case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "old mode") || strings.HasPrefix(line, "new mode") ||
			strings.HasPrefix(line, "similarity") ||
			strings.HasPrefix(line, "new file") || strings.HasPrefix(line, "deleted file") ||
			strings.HasPrefix(line, "Binary files"):
			h = nil

		default:
			return nil, fmt.Errorf("line %d: %q is neither a header nor a hunk line", i+1, line)
		}
	}
	return out, nil
}

// sides settles the file a diff section changes. git writes the path behind
// a/ on the --- line and behind b/ on the +++ line; a diff written with no
// prefixes writes the path itself on both. The reading that makes the two
// lines agree is the path, and a pair that agrees under neither reading is
// refused instead of guessed at.
func sides(from, to string) (string, bool, error) {
	from, to = column(from), column(to)
	stripped := [2]string{strings.TrimPrefix(from, "a/"), strings.TrimPrefix(to, "b/")}
	switch {
	case from == "" || to == "":
		return "", false, errors.New("the file has no name")
	case from == null && to == null:
		return "", false, errors.New("both sides are /dev/null")
	case to == null:
		return stripped[0], true, nil
	case from == null:
		return stripped[1], false, nil
	case stripped[0] == stripped[1]:
		return stripped[1], false, nil
	case from == to:
		return to, false, nil
	}
	return "", false, fmt.Errorf("the diff changes %s on one side and %s on the other", from, to)
}

// column drops the timestamp a diff may write in a second column after the
// path.
func column(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// parseHunk reads "@@ -old,count +new,count @@".
func parseHunk(line string) (hunk, error) {
	body := line[2:]
	end := strings.Index(body, "@@")
	if end < 0 {
		return hunk{}, fmt.Errorf("hunk header does not close")
	}
	fields := strings.Fields(body[:end])
	if len(fields) != 2 || !strings.HasPrefix(fields[0], "-") || !strings.HasPrefix(fields[1], "+") {
		return hunk{}, fmt.Errorf("hunk header %q", strings.TrimSpace(body[:end]))
	}
	old, err := firstNum(fields[0][1:])
	if err != nil {
		return hunk{}, err
	}
	next, err := firstNum(fields[1][1:])
	if err != nil {
		return hunk{}, err
	}
	return hunk{old: old, new: next}, nil
}

func firstNum(s string) (int, error) {
	if i := strings.Index(s, ","); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("hunk line number %q", s)
	}
	return n, nil
}

// apply runs every hunk against the file's current lines. A context or
// removed line that does not match is refused with its number.
func apply(name string, old []byte, hunks []hunk) ([]byte, error) {
	lines, endNL := splitLines(old)
	var out []string
	at := 0 // how far into lines we have copied

	for _, h := range hunks {
		start := h.old - 1
		if h.old == 0 {
			start = 0
		}
		if start < at || start > len(lines) {
			return nil, fmt.Errorf("%s: hunk at line %d does not fit a file of %d lines", name, h.old, len(lines))
		}
		out = append(out, lines[at:start]...)
		at = start

		for _, l := range h.lines {
			switch l[0] {
			case ' ', '-':
				if at >= len(lines) {
					return nil, fmt.Errorf("%s: hunk at line %d runs past the end of the file", name, h.old)
				}
				if lines[at] != l[1:] {
					return nil, fmt.Errorf("%s:%d: the file has %q where the diff expects %q", name, at+1, lines[at], l[1:])
				}
				at++
				if l[0] == ' ' {
					out = append(out, l[1:])
				}
			case '+':
				out = append(out, l[1:])
			}
		}
		if h.noEndNew {
			endNL = false
		} else if at >= len(lines) {
			endNL = true
		}
	}
	out = append(out, lines[at:]...)

	if len(out) == 0 {
		return nil, nil
	}
	text := strings.Join(out, "\n")
	if endNL {
		text += "\n"
	}
	return []byte(text), nil
}

// splitLines cuts a file into lines and reports whether it ended with a
// newline, which the joined result has to reproduce exactly.
func splitLines(b []byte) ([]string, bool) {
	if len(b) == 0 {
		return nil, false
	}
	s := string(b)
	nl := strings.HasSuffix(s, "\n")
	if nl {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n"), nl
}
