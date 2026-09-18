---
id: 2609131911
title: >-
  Share the refactor Workspace adapter between CLI and Session
status: "🔲"
model: sonnet
summary: >-
  cmd/mdsmith/rename.go's cliRenameWorkspace and
  pkg/mdsmith/refactor.go's sessionRefactorWorkspace each
  hand-write the same internal/refactor.Workspace seam over a
  transient internal/index.Index. Flagged by the 2026-09-13
  audit as tax.
---
# Share the refactor Workspace adapter between CLI and Session

## Goal

Replace the two independent `internal/refactor.Workspace`
implementations with one shared constructor.

Both `cmd/mdsmith` and `pkg/mdsmith` call it, so the
adapter shape changes in one place, not two.

## Background

The 2026-09-13 audit (see [the audit log][audit-log]) found:

- [cmd/mdsmith/rename.go][cli-rename]'s `cliRenameWorkspace`
  builds a transient [internal/index.Index][index] over
  discovered files, then implements `Resolve` by reading
  through it.
- [pkg/mdsmith/refactor.go][pkg-refactor]'s
  `sessionRefactorWorkspace` does the same thing for the
  `Session`-backed engine API.
- [go.md][go]'s DIP section names `internal/index` as "a peer
  support package both entry points may import" — so both
  call sites importing it is not a layering violation. The
  duplication is in the adapter code itself: the same
  "build an index from a resolved file list, then satisfy
  `Workspace.Resolve` by reading through it" logic is
  written twice.
- The two implementations aren't identical: the CLI variant
  additionally needs gitignore-aware `discovery.Discover`
  plus `git mv`/`FileOp.Execute` (see
  [plan/2607040822][2607040822]), while the `Session`
  variant is read-only. Any shared helper must keep that
  difference — this is a "factor out the common core, keep
  the divergent parts local" move, not a straight merge.

## Tasks

1. Read [cliRenameWorkspace][cli-rename] and
   `sessionRefactorWorkspace` in [pkg/mdsmith/refactor.go][pkg-refactor]
   side by side; list exactly which lines are identical
   (index construction + `Resolve`) versus which differ
   (discovery, write-back).
2. Design a shared constructor — e.g.
   `internal/index.NewWorkspace(files []string) internal/refactor.Workspace`,
   or a small helper type in `internal/refactor` itself —
   that owns the identical part.
3. Update `cliRenameWorkspace` and `sessionRefactorWorkspace`
   to build on the shared constructor, keeping their
   divergent discovery/write-back logic local.
4. `go build ./...` passes.
5. `go test ./...` passes, including
   [cmd/mdsmith/rename_unit_test.go][cli-rename-test] and
   [pkg/mdsmith/refactor_test.go][pkg-refactor-test].
6. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.

## Acceptance Criteria

- [ ] Only one place constructs an `internal/index.Index`-backed
      `internal/refactor.Workspace` from a file list; both the
      CLI and `pkg/mdsmith.Session` call it.
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
[2607040822]: 2607040822_refactor-move-rename-redesign.md
