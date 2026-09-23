# AGENTS.md - Context for AI Coding Assistants

## Persona & Expertise

You are an expert Systems Engineer specializing in Linux Kernel internals, the
Linux ABI, and systems programming in Go. You understand how system calls work,
the nuances of memory management, and the security implications of sandbox
escape vulnerabilities.

## Project Overview

gVisor is a user-space kernel, written in Go, that implements a substantial
portion of the Linux system surface. It provides an isolation boundary between
applications and the host kernel.

-   **Sentry:** The heart of gVisor; it acts as the "kernel" running the
    application.
-   **Gofer:** Handles file system operations to provide further isolation.
-   **runsc:** The OCI-compatible runtime executable.

## Tech Stack & Tooling

-   **Language:** Go (Golang).
-   **Build System:** Bazel (primary). Use `make` as a wrapper for common tasks.
-   **Platform:** Linux (x86_64, ARM64).

## Critical Development Commands

AI agents should use these commands to build, test, and verify:

-   **Build all targets:** `make build`
-   **Run unit tests:** `make tests`
-   **Run a specific test:** `make test TARGETS="//runsc:version_test"`

## Repository Structure

-   `/pkg/sentry`: The core "kernel" logic (process management, memory,
    syscalls).
-   `/pkg/abi`: Definitions of Linux constants and structures.
-   `/pkg/sentry/syscalls`: Implementation of individual Linux syscall handlers.
-   `/runsc`: Entry point for the OCI runtime.
-   `/tools`: Development and build utilities.

## The control plane's wire

`pkg/urpc` carries every control call between `runsc` and the sentry. Payloads
cross as ZAP, not JSON: `//tools/zap` reflects over each payload type at build
time and writes `zap.go` beside it, stating the offsets as constants, so
nothing reflects while a call is served. Reading `control.ExecArgs` back costs
641ns where `encoding/json` cost 5960ns.

-   **After changing a payload type, run `make zap`.** It empties every
    generated `zap.go` and rewrites it, which is necessary because a layout
    whose type moved no longer compiles -- and that is the point. A stale
    offset would be silent corruption, so it is a build failure instead.
-   **Append fields at the end and only at the end.** Reordering, inserting or
    retyping a field changes the wire for every peer.
-   **A payload is a struct that states a wire, or it is nothing.** `*struct{}`
    means a call with no argument or no result. A method whose payload is a
    bare string or int has no layout to state; give it a named type.
-   **A map, an interface, a uintptr and a type from another module have no
    offsets.** `make zap` refuses them by name. The answer is a change to the
    type: a map crosses as a list of pairs, a document crosses as its own
    bytes.

## Git & PR Guidelines

-   **Breaking Changes:** Any change to the ABI implementation must be verified
    against the equivalent Linux kernel behavior.
