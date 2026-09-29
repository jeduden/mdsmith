---
id: 2609131911
title: >-
  Share the refactor Workspace adapter between CLI and Session
status: "🔲"
model: sonnet
summary: >-
  cmd/mdsmith/rename.go's cliRenameWorkspace and
  pkg/mdsmith/refactor.go's sessionRefactorWorkspace both
  implement internal/refactor.Workspace over a transient
  internal/index.Index; their Incoming*Edges/Files
  pass-throughs are identical, though Resolve is not. Flagged
  by the 2026-09-13 audit as tax.
---
# Share the identical Workspace pass-through methods

## Goal

Replace the duplicated `IncomingAnchorEdges`,
`IncomingPathEdges`, `IncomingWikilinkEdges`, and `Files`
pass-throughs in `cliRenameWorkspace` and
`sessionRefactorWorkspace` with one shared implementation.

`Resolve` stays local to each type: they read from genuinely
different sources.

## Background

The 2026-09-13 audit (see [the audit log][audit-log]) found:

- [cmd/mdsmith/rename.go][cli-rename]'s `cliRenameWorkspace`
  and [pkg/mdsmith/refactor.go][pkg-refactor]'s
  `sessionRefactorWorkspace` both wrap an
  [internal/index.Index][index] to implement
  `internal/refactor.Workspace`.
- Their `IncomingAnchorEdges`, `IncomingPathEdges`,
  `IncomingWikilinkEdges`, and `Files` methods are
  byte-for-byte identical: each just forwards to the
  matching `Index` method.
- Their `Resolve` methods are not duplicates — they read from
  genuinely different sources. `cliRenameWorkspace.Resolve`
  reads from disk via `bytelimit.ReadFileLimited`, keyed by a
  workspace-relative path, honoring `rootDir` and `maxBytes`.
  `sessionRefactorWorkspace.Resolve` reads through the
  session's overlay-aware `Workspace.ReadFile`, keyed by a
  URI, substituting `overlaySource` for the file under edit.
  An initial version of this plan asked to unify `Resolve`
  too; review caught that this conflicted with the plan's own
  "no behavior change" criterion, since the two `Resolve`
  behaviors are not interchangeable. Scope corrected: this
  plan covers only the four identical pass-throughs.
- [go.md][go]'s DIP section names `internal/index` as "a peer
  support package both entry points may import" — so both
  types importing it is not itself a layering violation; the
  finding is the duplicated pass-through code, not the shared
  import.

## Tasks

1. Read [cliRenameWorkspace][cli-rename] and
   `sessionRefactorWorkspace` in [pkg/mdsmith/refactor.go][pkg-refactor]
   side by side to confirm the four pass-through methods stay
   byte-identical and `Resolve` stays the only divergent one.
2. Add a small embeddable helper — e.g. a type in
   `internal/index` that wraps an `*index.Index` and provides
   `IncomingAnchorEdges`, `IncomingPathEdges`,
   `IncomingWikilinkEdges`, and `Files` — that both
   `cliRenameWorkspace` and `sessionRefactorWorkspace` embed.
3. Update both types to embed the helper instead of
   hand-writing the four pass-throughs; keep each type's own
   `Resolve` method unchanged.
4. `go build ./...` passes.
5. `go test ./...` passes, including
   [cmd/mdsmith/rename_unit_test.go][cli-rename-test] and
   [pkg/mdsmith/refactor_test.go][pkg-refactor-test].
6. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.

## Acceptance Criteria

- [ ] `IncomingAnchorEdges`, `IncomingPathEdges`,
      `IncomingWikilinkEdges`, and `Files` are implemented
      once and shared by both `cliRenameWorkspace` and
      `sessionRefactorWorkspace`.
- [ ] `Resolve` stays a distinct method on each type; neither
      is asked to read the other's source.
- [ ] No behavior change: `mdsmith rename` and
      `Session.Rename`/`Session.Move` produce identical results
      before and after.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[go]: ../docs/development/architecture/go.md
[cli-rename]: ../cmd/mdsmith/rename.go
[cli-rename-test]: ../cmd/mdsmith/rename_unit_test.go
[pkg-refactor]: ../pkg/mdsmith/refactor.go
[pkg-refactor-test]: ../pkg/mdsmith/refactor_test.go
[index]: ../internal/index/index.go
