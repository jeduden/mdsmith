---
id: 2609271912
title: >-
  Share rename-mode detection between the CLI and the engine
status: "🔳"
model: sonnet
summary: >-
  cmd/mdsmith/rename.go's detectRenameMode and
  pkg/mdsmith/refactor.go's detectRenameKind both wrap the
  same refactor.FindHeadingLine / refactor.HasLinkRef pair
  in an identical switch. Flagged by the 2026-09-27 audit
  as tax.
---
# Share rename-mode detection between the CLI and the engine

## Goal

Collapse the shared *detection* core of the two host-side
rename-mode functions into one function. It lands on
[internal/refactor][refactor], the package both surfaces
already import. Each host keeps its own message formatting
and exit-code/error-value shape. Only the "which kind does
oldName match" decision moves.

## Background

The 2026-09-27 audit (see [the audit log][audit-log]) found
this duplication:

- [cmd/mdsmith/rename.go][cmd-rename]'s `detectRenameMode`
  and [pkg/mdsmith/refactor.go][pkg-refactor]'s
  `detectRenameKind` both call `refactor.FindHeadingLine`
  and `refactor.HasLinkRef` and switch on the identical
  both-match / heading-only / label-only / neither-match
  four-way outcome. The two functions then diverge on
  presentation: the CLI prints to `os.Stderr` and returns an
  exit code, plus a `move`-steering branch for path-shaped
  names that `pkg/mdsmith` doesn't have; `pkg/mdsmith`
  returns a Go `error`. Only the branch-detection core is
  duplicated, not the wrapping.
- [go.md][go]'s "Common violations to flag" calls out logic
  reimplemented per host surface instead of living once on
  the shared engine every host calls.
- [engine-api.md][engine-api] documents the public
  `pkg/mdsmith` Go API and its WASM bindings as mirroring
  the CLI one-to-one; a future refinement to the detection
  rule (e.g. a new match kind) applied to one copy and not
  the other would break that mirroring silently.

## Tasks

1. [x] Read [cmd/mdsmith/rename.go][cmd-rename]'s
   `detectRenameMode` and
   [pkg/mdsmith/refactor.go][pkg-refactor]'s
   `detectRenameKind` in full, plus their existing unit
   tests, and confirm both wrap the same four-way
   `isHeading`/`isLabel` branch on top of
   `refactor.FindHeadingLine` / `refactor.HasLinkRef`.
2. [x] Add a `refactor.DetectRenameKind(source []byte, oldName
   string) (kind string, ambiguous, found bool)` (or an
   equivalent small result type) to [internal/refactor][refactor]
   that returns only the detection outcome — no message
   text, no exit code — with a dedicated
   `TestDetectRenameKind` covering all four outcomes (both
   match, heading only, label only, neither).
3. Update `cmd/mdsmith/rename.go`'s `detectRenameMode` to call
   `refactor.DetectRenameKind` and keep its own
   `os.Stderr`/exit-code/`move`-steering wrapping around the
   result.
4. Update `pkg/mdsmith/refactor.go`'s `detectRenameKind` to
   call `refactor.DetectRenameKind` and keep its own
   `error`-returning wrapping around the result.
5. `go build ./...` passes.
6. `go test ./...` passes.
7. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.

## Acceptance Criteria

- [ ] The `isHeading`/`isLabel` detection switch exists in
      exactly one place: `internal/refactor`.
- [ ] `cmd/mdsmith rename` and `pkg/mdsmith`'s `Session.Rename`
      (and its WASM binding) produce the same user-visible
      messages and exit codes/errors as before the change —
      only the internal detection call changes.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[go]: ../docs/development/architecture/go.md
[engine-api]: ../docs/background/concepts/engine-api.md
[refactor]: ../internal/refactor/
[cmd-rename]: ../cmd/mdsmith/rename.go
[pkg-refactor]: ../pkg/mdsmith/refactor.go
