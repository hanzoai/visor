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

// patch applies a unified diff. Every file is verified and staged first, and
// only then renamed into place, so a diff that does not fit changes nothing.
func (d *daemon) patch(r *req) *rep {
	changes, err := parseDiff(string(r.data))
	if err != nil {
		return fail(r, err)
	}
	if len(changes) == 0 {
		return fail(r, fmt.Errorf("the diff names no file"))
	}

	type staged struct {
		dirfd int
		tmp   string
		base  string
		kill  bool
		path  string
	}
	var work []staged
	clean := func() {
		for _, s := range work {
			if !s.kill {
				unix.Unlinkat(s.dirfd, s.tmp, 0)
			}
			unix.Close(s.dirfd)
		}
	}

	for _, c := range changes {
		dirfd, base, err := d.root.parent(c.path)
		if err != nil {
			clean()
			return fail(r, err)
		}
		s := staged{dirfd: dirfd, base: base, kill: c.kill, path: c.path}
		if c.kill {
			if _, err := d.root.open(c.path, unix.O_PATH, 0); err != nil {
				unix.Close(dirfd)
				clean()
				return fail(r, err)
			}
			work = append(work, s)
			continue
		}

		old, mode, err := d.current(c.path)
		if err != nil {
			unix.Close(dirfd)
			clean()
			return fail(r, err)
		}
		next, err := apply(c.path, old, c.hunks)
		if err != nil {
			unix.Close(dirfd)
			clean()
			return fail(r, err)
		}
		s.tmp = stage(base, r.id)
		unix.Unlinkat(dirfd, s.tmp, 0)
		fd, err := d.root.openAt(dirfd, s.tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, mode)
		if err != nil {
			unix.Close(dirfd)
			clean()
			return fail(r, err)
		}
		f := os.NewFile(uintptr(fd), s.tmp)
		_, werr := f.Write(next)
		cerr := f.Close()
		if werr == nil {
			werr = cerr
		}
		if werr != nil {
			unix.Unlinkat(dirfd, s.tmp, 0)
			unix.Close(dirfd)
			clean()
			return fail(r, werr)
		}
		work = append(work, s)
	}

	reply := &rep{}
	for _, s := range work {
		if s.kill {
			if err := unix.Unlinkat(s.dirfd, s.base, 0); err != nil {
				clean()
				return fail(r, fmt.Errorf("delete %s: %w", s.path, err))
			}
		} else if err := unix.Renameat(s.dirfd, s.tmp, s.dirfd, s.base); err != nil {
			clean()
			return fail(r, fmt.Errorf("rename over %s: %w", s.path, err))
		}
		reply.entries = append(reply.entries, entry{name: s.path})
	}
	for _, s := range work {
		unix.Close(s.dirfd)
	}
	reply.size = uint64(len(work))
	return reply
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

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "--- "):
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
				return nil, fmt.Errorf("line %d: --- without +++", i+1)
			}
			from := diffPath(line[4:])
			to := diffPath(lines[i+1][4:])
			i++
			name, kill := to, false
			if to == null {
				name, kill = from, true
			}
			if name == null || name == "" {
				return nil, fmt.Errorf("line %d: the file has no name", i+1)
			}
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

		case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "old mode") || strings.HasPrefix(line, "new mode") ||
			strings.HasPrefix(line, "similarity") || strings.HasPrefix(line, "rename ") ||
			strings.HasPrefix(line, "new file") || strings.HasPrefix(line, "deleted file") ||
			strings.HasPrefix(line, "Binary files"):
			h = nil

		default:
			return nil, fmt.Errorf("line %d: %q is neither a header nor a hunk line", i+1, line)
		}
	}
	return out, nil
}

// diffPath drops a timestamp column and the a/ or b/ prefix git writes.
func diffPath(s string) string {
	if i := strings.IndexAny(s, "\t"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == null {
		return s
	}
	for _, p := range []string{"a/", "b/"} {
		if strings.HasPrefix(s, p) {
			return s[len(p):]
		}
	}
	return s
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
