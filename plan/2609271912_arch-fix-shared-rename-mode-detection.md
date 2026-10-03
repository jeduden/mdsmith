---
id: 2609271912
title: >-
  Share rename-mode detection between the CLI and the engine
status: "✅"
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

Collapse the rename dispatch the two hosts duplicated into
one function. It lands on [internal/refactor][refactor],
the package both surfaces already import. Each host keeps
its own message formatting and exit-code/error-value shape.
The plan first moved only the "which kind does oldName
match" decision; review showed the hosts had also drifted on
the kind → planner step, so `refactor.Rename` now owns
detection and dispatch (tasks 8–10).

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
3. [x] Update `cmd/mdsmith/rename.go`'s `detectRenameMode` to call
   `refactor.DetectRenameKind` and keep its own
   `os.Stderr`/exit-code/`move`-steering wrapping around the
   result.
4. [x] Update `pkg/mdsmith/refactor.go`'s `detectRenameKind` to
   call `refactor.DetectRenameKind` and keep its own
   `error`-returning wrapping around the result.
5. [x] `go build ./...` passes.
6. [x] `go test ./...` passes.
7. [x] `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.
8. [x] Review round 1 deviation: the hosts still each mapped
   kind → planner and handled empty results, and they had
   already drifted. On a same-name heading or a missing
   explicit label, `Session.Rename` returned an empty plan
   with a nil error, while the CLI exited 1. Move the whole
   dispatch into `refactor.Rename(ws, fileKey, source, kind,
   oldName, newName)`. It returns typed outcomes:
   `ErrAmbiguousRename`, `ErrNoRenameTarget`,
   `ErrNothingToRename`, `MissingSymbolError`, and
   `InvalidRenameKindError`. Each host keeps only its own
   message and exit-code mapping (`renameExitCode`,
   `renameError`).
9. [x] Make the kind a typed `refactor.RenameKind`
   (`KindHeading`, `KindLabel`). Validate both hosts'
   explicit selectors through `refactor.ParseRenameKind`.
   The detection result becomes `(RenameKind, line, error)`
   in place of the `(kind, ambiguous, found)` triple, so no
   impossible combination can be expressed. Detection now
   lives in the unexported `detectRenameKind`.
10. [x] Reuse the heading line detection already found, so an
    auto-detected heading rename searches for the heading
    once rather than twice. Make `HasLinkRef` and
    `FindHeadingLine` unexported (`hasLinkRef`,
    `findHeadingLine`), since neither has callers outside
    `internal/refactor`.
11. [x] Review round 2: a label renamed to the spelling
    every occurrence already has rewrote the file with
    identical bytes and exited 0, while the heading
    equivalent exited 1. `ErrNothingToRename` now comes as
    a `NothingToRenameError{Kind, Name}` for both kinds, so
    the CLI exits 1 with `nothing to rename for label
    "docs"` and `Session.Rename` errors with the same text.

## Acceptance Criteria

- [x] The `isHeading`/`isLabel` detection switch exists in
      exactly one place: `internal/refactor`.
- [x] `cmd/mdsmith rename` produces the same user-visible
      messages and exit codes as before the change, except
      that a no-op label rename now exits 1 like a no-op
      heading rename (task 11).
      `pkg/mdsmith`'s `Session.Rename` (and its WASM binding)
      keeps its error text, but now errors where the CLI
      exits 1 instead of returning an empty plan: a
      same-name heading or label rename and a missing
      explicit label.
- [x] `go test ./...` is green.
- [x] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[go]: ../docs/development/architecture/go.md
[engine-api]: ../docs/background/concepts/engine-api.md
[refactor]: ../internal/refactor/
[cmd-rename]: ../cmd/mdsmith/rename.go
[pkg-refactor]: ../pkg/mdsmith/refactor.go
