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

The host keeps `mine` and speaks ZAP on it. When the host closes its end,
`execd`'s read returns end of file, every pty is killed, every watch is closed
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
| `git` | 12 | `argv`, `dir`, `timeout` | `exit`, `data`, `log` |

A reply whose `err` is set did not happen. `err` is a sentence, not a code:
`../outside/secret: invalid cross-device link`, `no answer within 200ms`,
`src/main.go:2: the file has "x" where the diff expects "two"`.

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

The ledger records each completed action's reply under its id. A repeated id
is answered from that record and the effect is **not** run again: if the host
loses an answer to `Exec{id: 7821, argv: ["cargo", "test"]}`, asking again
returns the recorded result rather than a second test run. The same holds for
`git commit`, a write and a patch.

The table is bounded by count: the last 1024 replies are held, and the 1024
ids before those are kept as bare marks. An id whose result was evicted is
refused (`action N completed and its result is no longer held`) rather than
run twice. Past that window an id is forgotten and a repeat of it runs, so a
host that may replay should not reuse an id older than the window.

An id still running is refused with `action N is already running`. An id of
zero is refused: every action carries one.

## The workspace is the boundary

Every path in a request is resolved relative to `-workspace` with
`openat2(RESOLVE_BENEATH | RESOLVE_NO_MAGICLINKS)`. The kernel does the
refusing, not a string check, so all of these fail:

- `../outside/secret`, and any `..` at all — even one that would stay inside
- `/etc/passwd`, and any absolute path
- a symlink whose target leaves the tree
- an absolute symlink, even one aimed back inside the workspace, because
  resolution would restart at `/`

A relative symlink that stays beneath the workspace is followed normally.

`exec` and `git` take their working directory the same way, and run in the
resolved directory. `git` also gets `GIT_CEILING_DIRECTORIES` set to the
workspace, so a repository search cannot walk above it, and
`GIT_DISCOVERY_ACROSS_FILESYSTEM=0`. `git` takes `argv` only: a command
string for a shell is refused.

## Writing is atomic, not durable

`write` and `patch` write a temporary file beside the target and rename over
it, so a reader sees the old bytes or the new ones and never a half-written
file. `patch` verifies and stages **every** file in the diff before renaming
any of them: a diff whose second file does not fit changes nothing.

There is no `fsync`. The workspace is served to the sandbox by the host, which
caches locally and persists to s3; making bytes durable is the host's job, and
an fsync per write inside the cell buys nothing and can cost seconds.

The diff `patch` reads is a plain unified diff: `---`/`+++` headers with the
`a/` and `b/` prefixes git writes, `@@` hunks, `/dev/null` on one side for a
file created or deleted, and `\ No newline at end of file`. Anything else is
refused with the line number. A context or removed line that does not match
the file is refused with what was there and what the diff expected.

## Bounds

| Bound | Value | What happens past it |
|---|---|---|
| one frame | 128 KiB | refused with the size |
| one read | 96 KiB | refused; ask for a range |
| stdout, stderr per `exec` | 48 KiB each | kept bytes plus `err` saying how many were dropped |
| one listing | 4096 entries | page it with `off` and `length` |
| ledger | 1024 results, 1024 marks | an evicted id is refused, not re-run |

A listing is sorted by name, because a page is only meaningful against a
stable order.

## Build

    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/execd
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/execd

Linux only: it is `openat2`, `inotify` and a pty, and there is nothing to
pretend about on another kernel.

## Tests

The tests need no visor and no root. They make a socketpair in-process, run
the daemon on one end and speak ZAP on the other — the same code path as FD 3
under runsc:

    go test ./cmd/execd
