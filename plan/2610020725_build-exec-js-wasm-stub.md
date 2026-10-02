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
   `internal/refactor` is tagged `!wasm`. Add
   `fileop_exec_wasm.go`, a stub that returns an error, with a
   wasm test.

## Acceptance Criteria

- [x] `GOOS=js GOARCH=wasm go build ./...` exits 0.
- [x] CI runs that whole-module wasm build on every PR.
- [x] Unix and Windows behavior of `runRecipe` is
      unchanged.
