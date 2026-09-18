---
id: 2609131913
title: >-
  Add unit tests for untested move/rename helper functions
status: "🔲"
model: sonnet
summary: >-
  internal/refactor/move.go's recomputeToken, encodePathToken,
  pathEdit, and countFilesWithStem, plus cmd/mdsmith's
  applyEditsToFile, looksLikePath, firstPathish, and
  resolveWriteMode, have no dedicated test symbol by name —
  only indirect coverage via behavior-level tests. Flagged by
  the 2026-09-13 audit as tax (internal/refactor/move.go) and
  nice-to-have (cmd/mdsmith).
---
# Add unit tests for untested move/rename helper functions

## Goal

Give each listed helper a dedicated unit test by name, per
[tests.md][tests]'s per-function test rule, without changing
any behavior.

## Background

The 2026-09-13 audit (see [the audit log][audit-log])
found these functions untested by name.

Each is covered only indirectly, through behavior-level
tests:

- [internal/refactor/move.go][move]: `recomputeToken`,
  `encodePathToken`, `pathEdit`, `countFilesWithStem` — tax,
  since none sits on a public surface (`rule.Rule` method, LSP
  capability handler, CLI subcommand entry).
- [cmd/mdsmith/move.go][cli-move]: `applyEditsToFile`.
- [cmd/mdsmith/rename.go][cli-rename]: `looksLikePath`,
  `firstPathish`, `resolveWriteMode` — nice-to-have, since
  these are small, low-risk, and already covered indirectly by
  `TestRunMove_Success`, `TestRunRename_MoveIntentGuard`, and
  `TestWriteFilePreservingMode_*`.

Both groups are the same class of fix — add a missing unit
test — so this plan covers both rather than splitting by
severity.

## Tasks

1. Read each function's current behavior and existing indirect
   coverage in [internal/refactor/move_test.go][move-test],
   [internal/refactor/move_coverage_test.go][move-coverage-test],
   [cmd/mdsmith/move_unit_test.go][cli-move-test], and
   [cmd/mdsmith/rename_unit_test.go][cli-rename-test].
2. Add `TestRecomputeToken`, `TestEncodePathToken`,
   `TestPathEdit`, and `TestCountFilesWithStem` as table-driven
   tests in `internal/refactor/move_test.go` (or
   `move_coverage_test.go`, matching the existing split).
3. Add `TestApplyEditsToFile` in
   `cmd/mdsmith/move_unit_test.go`.
4. Add `TestLooksLikePath`, `TestFirstPathish`, and
   `TestResolveWriteMode` in `cmd/mdsmith/rename_unit_test.go`.
5. `go build ./...` passes.
6. `go test ./...` passes.
7. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.

## Acceptance Criteria

- [ ] Each of the eight functions listed above has a dedicated
      test symbol named after it in a sibling `*_test.go` file.
- [ ] No production code changes; behavior is unchanged.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[move]: ../internal/refactor/move.go
[move-test]: ../internal/refactor/move_test.go
[move-coverage-test]: ../internal/refactor/move_coverage_test.go
[cli-move]: ../cmd/mdsmith/move.go
[cli-move-test]: ../cmd/mdsmith/move_unit_test.go
[cli-rename]: ../cmd/mdsmith/rename.go
[cli-rename-test]: ../cmd/mdsmith/rename_unit_test.go
