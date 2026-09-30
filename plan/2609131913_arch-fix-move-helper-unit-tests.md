---
id: 2609131913
title: >-
  Add unit tests for untested move.go helper functions
status: "🔲"
model: sonnet
summary: >-
  internal/refactor/move.go's recomputeToken, encodePathToken,
  pathEdit, and countFilesWithStem carry real path-rewriting
  logic with no dedicated test symbol by name — only indirect
  coverage via the TestMove_* behavior suite. Flagged by the
  2026-09-13 audit as tax.
---
# Add unit tests for untested move.go helper functions

## Goal

Give each of the four listed helpers in
`internal/refactor/move.go` a dedicated unit test by name.

This follows [tests.md][tests]'s per-function test rule,
without changing any behavior.

## Background

The 2026-09-13 audit (see [the audit log][audit-log])
found these functions untested by name.

Each is covered only indirectly, through the `TestMove_*`
behavior suite:

- [internal/refactor/move.go][move]: `recomputeToken`,
  `encodePathToken`, `pathEdit`, `countFilesWithStem` — tax,
  since none sits on a public surface (`rule.Rule` method, LSP
  capability handler, CLI subcommand entry). All four carry
  real path-rewriting or -matching logic, not boilerplate.

A related, smaller finding on `cmd/mdsmith` was scoped out
of this plan on review.

`looksLikePath`, `firstPathish`, and `resolveWriteMode`
([cmd/mdsmith/rename.go][cli-rename]) lack a test
symbol by name.

Each is small and low-risk. Each is also covered
indirectly by `TestRunRename_MoveIntentGuard` and
`TestWriteFilePreservingMode_*`.

Adding named tests for them is optional cleanup, not
required to close this plan.

The finding also named `applyEditsToFile` in
[cmd/mdsmith/move.go][cli-move]. That part is closed.
Its code now lives in `computePlanWrite` and
`commitPlan` in [cmd/mdsmith/planapply.go][cli-planapply].
Each has a named test in
`cmd/mdsmith/planapply_unit_test.go`.

## Tasks

1. Read each function's current behavior and existing indirect
   coverage in [internal/refactor/move_test.go][move-test] and
   [internal/refactor/move_coverage_test.go][move-coverage-test].
2. Add `TestRecomputeToken`, `TestEncodePathToken`,
   `TestPathEdit`, and `TestCountFilesWithStem` as table-driven
   tests in `internal/refactor/move_test.go` (or
   `move_coverage_test.go`, matching the existing split).
3. `go build ./...` passes.
4. `go test ./...` passes.
5. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.
6. Optional stretch, not required to close this plan: add
   `TestLooksLikePath`, `TestFirstPathish`, and
   `TestResolveWriteMode` in `cmd/mdsmith/rename_unit_test.go`.

## Acceptance Criteria

- [ ] Each of `recomputeToken`, `encodePathToken`, `pathEdit`,
      and `countFilesWithStem` has a dedicated test symbol
      named after it in `internal/refactor/move_test.go` or
      `move_coverage_test.go`.
- [ ] No production code changes; behavior is unchanged.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[move]: ../internal/refactor/move.go
[move-test]: ../internal/refactor/move_test.go
[move-coverage-test]: ../internal/refactor/move_coverage_test.go
[cli-move]: ../cmd/mdsmith/move.go
[cli-planapply]: ../cmd/mdsmith/planapply.go
[cli-rename]: ../cmd/mdsmith/rename.go
