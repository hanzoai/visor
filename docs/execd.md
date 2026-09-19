# execd — the execution daemon inside the sandbox

`execd` is the small program that runs inside a visor sandbox and does what a
coding agent asks of an operating system: run a program, drive a pty, read and
write files, watch them, apply a diff, run git. It holds no model, no
conversation, no plan and no credential. Those live outside the sandbox and
reach it as actions (HIP-1330).

    THINK  (outside)                    DO  (inside)
    /v1/dev + dev core   ── action ──▶  execd  ──▶  exec, pty, files, git
                         ◀── result ──

## The FD 3 contract

`execd` does not listen, does not connect, and has no path in any filesystem.
It is handed a descriptor that is already connected: **FD 3**, one end of a
`AF_UNIX`/`SOCK_SEQPACKET` socketpair the host created before the sandbox
started. A sequenced-packet socket keeps message boundaries, so one packet is
one ZAP frame and there is no length prefix and no framing layer.

At startup `execd` refuses anything that is not that contract:

| Check | Refusal |
|---|---|
| `SO_TYPE` is `SOCK_SEQPACKET` | `descriptor 3 is socket type N, not SOCK_SEQPACKET` |
| `getpeername` succeeds (connected) | `descriptor 3 is not connected` |
| `-workspace` given and openable | `-workspace is required` |

    execd -workspace /workspace        # fd defaults to 3
    execd -workspace /workspace -fd 3

A frame is at most 128 KiB. A larger request cannot be received, and is
answered with the size that arrived instead of being truncated in silence; a
reply that would not fit is refused the same way. That is why a read takes a
range and a listing takes a page.

### How the host makes the descriptor

```go
fds, _ := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
mine, theirs := fds[0], fds[1]

cmd := exec.Command("runsc", "run", "-bundle", bundle, id)
cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(theirs), "execd")} // becomes FD 3
```

`ExtraFiles[0]` is FD 3 in the child. `runsc` passes the descriptors it was
given through to the sandboxed process, so the OCI config runs `execd` with
that same descriptor:

```json
{
  "process": {
    "args": ["/sbin/execd", "-workspace", "/workspace"],
    "cwd": "/"
  }
}
```

Two things to keep true:

- The descriptor must not carry `FD_CLOEXEC` at the moment of exec. Go clears
  it for `ExtraFiles`; if you dup by hand, use `unix.Dup3(fd, 3, 0)`.
- Nothing else may be handed in below FD 3, or the numbering shifts. `execd`
  takes `-fd` for the case where the host cannot place it at 3.

`execd` sets `FD_CLOEXEC` on the descriptor as it starts. The socket crosses
the exec that starts `execd` and no other: a command `execd` runs gets its
stdio, and cannot read a request meant for `execd` or answer one in its name.

The host keeps `mine` and speaks ZAP on it. A zero-length packet is a packet
like any other and asks for nothing — it is the socket, not a length, that says
the host has hung up. When it has, every pty is killed, every watch is closed
and the process exits.

## The protocol

One ZAP message per packet. Every request carries an **action id**, and every
reply names the id it answers. Requests are carried out concurrently, so
replies can arrive in any order — match on the id, never on arrival order.

### Ops

| Op | Code | Takes | Answers |
|---|---|---|---|
| `exec` | 1 | `argv` or `command`, `dir`, `env`, `timeout`, `data` as stdin | `exit`, `data` (stdout), `log` (stderr) |
| `spawn` | 2 | `argv` or `command`, `dir`, `env`, `cols`, `rows` | `handle` |
| `pty.write` | 3 | `handle`, `data` | `size` written |
| `resize` | 4 | `handle`, `cols`, `rows` | `handle` |
| `kill` | 5 | `handle`, `signal` | `handle` |
| `read` | 6 | `path`, `off`, `length` | `data`, `size`, `mode`, `mtime` |
| `write` | 7 | `path`, `data`, `mode` | `size`, `mode` |
| `stat` | 8 | `path` | `size`, `mode`, `mtime`, `flags` |
| `list` | 9 | `path`, `off` (first index), `length` (count) | `entries`, `size` (the whole count) |
| `watch` | 10 | `path` | `handle`, `path` (resolved) |
| `patch` | 11 | `data` (a unified diff) | `entries` (the files changed), `size` |
| `git` | 12 | `argv`, `dir`, `env`, `timeout` | `exit`, `data`, `log` |

`mode` is permission bits — `0644`, `0755` — in a request and in every reply
that carries one. Whether a path is a directory is `flags`, not the mode.

### A reply that did not happen, and one that did

`err` is a sentence, not a code: `away/secret: invalid cross-device link`,
`no answer within 200ms`, `src/main.go:2: the file has "x" where the diff
expects "two"`. There are two kinds of it, and which one it is decides what a
repeat of the id does.

- **Refused.** The request was not carried out: a path outside the workspace, a
  diff that does not fit, `argv` and `command` both set, a handle that is gone,
  a frame over the bound, a program that could not be started. Nothing changed,
  so nothing is recorded and the same id sent again runs.
- **What came of it.** The action ran and this is its outcome: a nonzero
  `exit`, `killed by SIGKILL`, `no answer within 200ms` for a command its
  deadline killed and that may have left half its work behind. It is recorded
  under its id like any other answer. Running it again is a new action and
  takes an id of its own.

### Kinds

A reply is one of four kinds. Only `reply` answers a request; the rest are
pushed by a handle the host already holds, under the action id that created
it.

| Kind | Code | Meaning |
|---|---|---|
| `reply` | 0 | the answer to the request with this id |
| `data` | 1 | pty output |
| `event` | 2 | a watch event: `path` is the name, `mask` the inotify bits |
| `exit` | 3 | the spawned process ended: `exit`, and `err` if a signal ended it |

### An action happens once

The ledger records the reply of every action that ran, under its id. A repeated
id is answered from that record and the effect is **not** run again: if the
host loses an answer to `Exec{id: 7821, argv: ["cargo", "test"]}`, asking again
returns the recorded result rather than a second test run. The same holds for
`git commit`, a write and a patch. A refused request is not recorded, because
nothing happened.

The table is bounded twice, by count and by bytes: the newest 1024 replies are
held while they take no more than 8 MiB together, and the 1024 ids evicted
before those are kept as bare marks. An id whose result was evicted is refused
(`action N completed and its result is no longer held`) rather than run twice.
Past that window an id is forgotten and a repeat of it runs, so a host that may
replay should not reuse an id older than the window.

An id still running is refused with `action N is already running`. An id of
zero is refused: every action carries one.

## The workspace is the boundary

Every path in a request is resolved relative to `-workspace` with
`openat2(RESOLVE_BENEATH | RESOLVE_NO_MAGICLINKS)` — for a write and a patch as
much as for a read, and on the path as it was written rather than on a cleaned
copy of it, so one string cannot mean two places. The kernel does the refusing,
not a string check:

- `../outside/secret`, and any `..` that would leave the workspace
- `/etc/passwd`, and any absolute path
- a symlink whose target leaves the tree, and any path through one
- an absolute symlink, even one aimed back inside the workspace, because
  resolution would restart at `/`

A `..` that stays beneath the workspace resolves normally, so `src/../src/main.go`
is `src/main.go`; so does a relative symlink that stays beneath it.

`exec` and `git` take their working directory the same way, and run in the
resolved directory. For `git`, `dir` is the only thing in a request that says
where it works:

- Before the subcommand, `argv` takes `-c` and the options that change how git
  reads and prints: `-P`, `--no-pager`, `--no-optional-locks`,
  `--no-replace-objects` and the `--*-pathspecs` switches. `-C`, `--git-dir`,
  `--work-tree`, `--exec-path` and any option not listed are refused; one that
  is not listed might take a value and hide the option after it.
- A `GIT_*` variable in `env` is refused. `GIT_DIR`, `GIT_WORK_TREE` or a
  `GIT_CONFIG_*` would name a place of the request's choosing; `-c` says how
  git is configured.
- git reads the repository's configuration, the system's and what `-c` states.
  `GIT_CONFIG_GLOBAL` is `/dev/null`, so a `HOME` in `env` picks no file.

After the subcommand every word is git's own: `git log -C` is copy detection,
and a path a subcommand is given, like the directory of `git init <dir>`, is
git's to resolve, as a path in `exec`'s `argv` is.

`git` gets `GIT_CEILING_DIRECTORIES` set to the workspace's **parent**, not to
the workspace. git stops a repository search at a ceiling directory on the way
up, and a ceiling equal to the directory the search starts from is never on
that way — so a ceiling of the workspace itself would leave a request that runs
at the workspace root free to find, read and write a repository above it. With
the parent as the ceiling, the search ends at the workspace, from the root and
from any directory under it. What it finds there git follows as it always
does: a `.git` file or a `core.worktree` in the workspace that names a place
outside it takes git there. `GIT_DISCOVERY_ACROSS_FILESYSTEM=0` is set as well.
`git` takes `argv` only: a command string for a shell is refused.

A child starts from a declared environment — `PATH`, `LANG`, `TERM` — or from
the `env` the request names, and never from the daemon's own, so nothing the
host handed `execd` reaches a command unasked.

## Writing is atomic, not durable

`write` and `patch` fill a staged file beside the target and rename over it, so
a reader sees the old bytes or the new ones and never a half-written file. The
staged name is `.execd` and a random suffix — a name no request can name, so a
file the workspace already holds is never in the way of a write.

`write` sets the mode: the one the request names, or the one the file already
has, or `0644` for a file that is new. The reply names the mode the file ended
up with, not the one that was asked for.

`patch` is all or nothing. Every file in the diff is read, its hunks applied
and the result staged before anything is moved, and a file that was already
there is kept on a second hard link until every move has landed. So a diff
whose second file does not fit changes nothing, and a move that fails puts back
the moves before it. A diff that names one file twice is refused, as is one
that gives two names for one file, and one that would delete a directory.

There is no `fsync`. The workspace is served to the sandbox by the host, which
caches locally and persists to s3; making bytes durable is the host's job, and
an fsync per write inside the cell buys nothing and can cost seconds.

The diff `patch` reads is a plain unified diff: `---`/`+++` headers, `@@`
hunks, `/dev/null` on one side for a file created or deleted, and `\ No newline
at end of file`. Anything else is refused with the line number.

The path is the reading that makes the two header lines agree: `a/src/main.go`
and `b/src/main.go` are both `src/main.go`, while a diff written with no
prefixes names the path it writes even when that path starts with `a/`. A pair
that agrees under neither reading is refused instead of guessed at, and so is a
`rename from`/`rename to` pair, which a diff of hunks cannot express — send the
delete and the create. A context or removed line that does not match the file
is refused with what was there and what the diff expected.

## Bounds

| Bound | Value | What happens past it |
|---|---|---|
| one frame | 128 KiB | refused with the size |
| one read | 96 KiB | refused; ask for a range |
| stdout, stderr per `exec` | 48 KiB each | kept bytes plus `err` saying how many were dropped |
| one listing | what a frame holds | keep going from `off` |
| ledger | 1024 results, 8 MiB, 1024 marks | an evicted id is refused, not re-run |

A listing is sorted by name, because a page is only meaningful against a stable
order, and answers with the entries from `off` that fit in one frame. `size` is
the whole directory's count, so a host pages by advancing `off` by the number
of entries it got. A read whose offset is past the end of the file is refused
rather than answered with no bytes.

## Build

    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/execd
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/execd

Linux only: it is `openat2`, `inotify` and a pty, and there is nothing to
pretend about on another kernel.

## Tests

The tests need no visor and no root. They make a socketpair in-process, hand
the daemon one end without `FD_CLOEXEC`, as runsc hands over FD 3, and speak
ZAP on the other — the same code path as FD 3 under runsc:

    go test ./cmd/execd
