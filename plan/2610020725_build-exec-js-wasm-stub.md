---
id: 2610020725
title: >-
  Make internal/build compile under GOOS=js GOARCH=wasm
status: "✅"
model: sonnet
summary: >-
  `GOOS=js GOARCH=wasm go build ./...` fails on main because
  internal/build/exec.go calls configureProcessGroup,
  killGroup, and afterStart, which only exec_unix.go and
  exec_windows.go define. Add a stub for every other target
  and gate the whole-module wasm build in CI so the break
  cannot come back.
---
# Make internal/build compile under GOOS=js GOARCH=wasm

## Goal

`GOOS=js GOARCH=wasm go build ./...` succeeds, so the
module builds for wasm as a whole and not only through
`./cmd/mdsmith-wasm`.

## Background

Review of PR #876 found this failure. The same error
shows on `origin/main`. It is old:

```text
internal/build/exec.go:129:2: undefined: configureProcessGroup
internal/build/exec.go:159:3: undefined: killGroup
internal/build/exec.go:180:20: undefined: afterStart
```

[exec_unix.go](../internal/build/exec_unix.go) has the
build tag `unix`, and
[exec_windows.go](../internal/build/exec_windows.go) has
`windows`. `js`, `wasip1`, and `plan9` match neither tag,
so the three hooks are missing on those targets. CI does
not catch this. The wasm job in
[ci.yml](../.github/workflows/ci.yml) only vets and builds
`./cmd/mdsmith-wasm`, which never imports
`internal/build`.

## Tasks

1. Add a failing check: a CI step (or an
   `mdsmith-release` subcommand, per
   [release-tooling.md](../docs/development/release-tooling.md))
   that runs `GOOS=js GOARCH=wasm go build ./...`.
2. Add `internal/build/exec_other.go` with the tag
   `!unix && !windows`. In it, `configureProcessGroup` is
   a no-op, `afterStart` returns nil, and `killGroup`
   kills only the leader with `cmd.Process.Kill()`.
3. Add a `js && wasm` unit test for each stub, or add
   them to the wasm test runner from plan 2610020045.
4. Confirm `go build ./...` still passes on
   linux, darwin, and windows.
5. Added during implementation: the whole-module wasm build
   also failed in `cmd/mdsmith` because `FileOp.Execute` in
   `internal/refactor` is tagged `!wasm`. Route the one
   call through `executeFileOp` in `cmd/mdsmith`, with a
   `fileop_exec_wasm.go` stub that returns an error and a
   wasm test. (Round 1 put the stub in `internal/refactor`;
   round 2 moved it, see item 8.)
6. Added in review round 1. Gate test files and other
   targets too, since `go build` skips `_test.go`. CI now
   vets `./...` for js/wasm, wasip1, and plan9. It also
   runs golangci-lint for js/wasm and windows. To pass,
   the FIFO tests move from `!windows` to `unix`, and
   `unixProcessAlive` gets a `!unix` stub. The wasip1
   spike guest reads input via `keepAlive`. `gracePeriod`
   moves to `exec_unix.go`, and the docs now say Windows
   kills with no grace wait. `CloseHandle` gets an
   explicit error check. `newSessionProxy` is split per
   method.
7. Added in review round 1: a stub in `internal/refactor`
   dropped the compile-time guard on wasm `Execute`
   calls, so a type-check test (`execguard_test.go`)
   restored it.
8. Added in review round 2. The stub moved out of
   `internal/refactor` into `cmd/mdsmith`, the only
   caller, which the bridge never links. `FileOp.Execute`
   is again undefined under wasm, so the compiler rejects
   any wasm-reachable call and the guard test is gone.
   `TestMain` moves to a `!wasm` file so the cmd/mdsmith
   wasm test runs under Node. The session proxy's
   `dispose()` releases the other method funcs, so a
   disposed session no longer stays pinned. A js/wasm
   test ties the proxy's keys to `sessionMethodNames`.
9. Added in review round 2. After a timeout kill,
   `runRecipe` waits at most `reapWait` (5 s). If the
   recipe is still running, it kills the leader and
   waits at most `reapWait` again, then returns. So a
   Windows run with no Job Object, or a plan9 survivor
   holding the output pipe, can no longer hang mdsmith.

## Acceptance Criteria

- [x] `GOOS=js GOARCH=wasm go build ./...` exits 0.
- [x] CI runs that whole-module wasm build on every PR.
- [x] Unix and Windows kill signals in `runRecipe` are
      unchanged; only the post-kill wait is now bounded.
- [x] `go vet ./...` (tests included) passes for js/wasm,
      wasip1, and plan9, and CI gates it.
- [x] golangci-lint passes for js/wasm and windows, and
      CI gates it.
- [x] No package the wasm bridge links uses
      `FileOp.Execute`, enforced by the compiler.
