---
id: 2609061915
title: >-
  Add unit tests for the untested rename/move helpers
status: "🔲"
model: haiku
summary: >-
  21 private helpers in internal/refactor and its
  cmd/mdsmith and pkg/mdsmith callers have no unit test
  named after them, only scenario tests through their
  callers. Re-filed from closed PR #839 and re-checked
  against main on 2026-09-29.
---
# Add unit tests for the untested rename/move helpers

## Goal

Give each function below its own unit test by name, per
[tests.md][tests]'s "every function has a dedicated unit
test" rule.

## Background

Closed PR #839 first filed this plan with its 2026-09-06
architecture audit of the `internal/refactor` rename/move
engine and its callers. That PR closed, so the plan never
reached main. This copy is re-filed from it. Every item was
re-checked against main on 2026-09-29, and these 21 functions
still have no test named after them:

- [internal/refactor/fileop_exec.go][fileop-exec]:54,65 —
  `gitTracked`, `gitMove`. Only the `TestFileOpExecute_*`
  tests reach them, through `FileOp.Execute`.
- [internal/refactor/rename.go][rename]:169,254,300,329,393,472,508 —
  `labelConflict`, `linkRefEdits`, `refUseEditsInBody`,
  `refUseEdit`, `linkTextBounds`, `bodyNewlineCount`, and the
  private `bodyAndFMOffset`. `TestLinkRef_LabelConflict` calls
  `LinkRef`, not `labelConflict`. The
  `TestRefUseEditsInBodyIgnoresOtherLabels` and
  `TestRefUseEditCollapsedReferenceLabel` tests live in
  `internal/lsp` and drive a `textDocument/rename` request
  through a server harness. They are integration tests, not
  unit tests next to the source. `TestBodyAndFMOffset` calls
  only the exported `BodyAndFMOffset` at line 134, a one-line
  wrapper around the private function.
- [cmd/mdsmith/move.go][cmd-move]:179 — `applyEditsToFile`.
- [cmd/mdsmith/rename.go][cmd-rename]:219,276,304,324,341,350,448 —
  `buildWorkspace`, `detectRenameMode`, `headingPlan`,
  `linkRefPlan`, `looksLikePath`, `firstPathish`,
  `resolveWriteMode`.
- [pkg/mdsmith/refactor.go][pkg-refactor]:104,122,168,184 —
  `detectRenameKind`, `toRefactorPlan`,
  `(*sessionRefactorWorkspace).Resolve`, and
  `(*Session).buildRefactorWorkspace`.

Each is reached only through a caller's scenario test
(`TestMove_*`, `TestLinkRef_*`, `TestRunRename_*`,
`TestRunMove_*`, `TestSession_Rename*`, `TestSession_Move*`).
None is a public surface by itself, so all are `tax`, not
`blocker`.

## Scope changes since PR #839

Left out, with the reason:

- `validRefDefMatches`: `TestValidRefDefMatchesNoRefDefsCheapNoParse`
  in [perf_test.go][perf-test] already calls it directly.
- `recomputeToken`, `encodePathToken`, `pathEdit`,
  `countFilesWithStem` in `internal/refactor/move.go`: open
  PR #842 files plan 2609131913 for these four. If #842
  closes without merging, add them back here.
- `resolveFileEdits` and `writeRewrittenFile`: they existed
  only on PR #839's branch, split from `applyEditsToFile`.
  Main still has `applyEditsToFile`, listed above.
- `sortEdgesBySource` in `internal/index/index.go`: open PR
  #838 adds `TestSortEdgesBySource_NoReflectSort` and
  `TestSortEdgesBySource_TiesPreserveInsertionOrder`. If #838
  closes without merging, add it back here.
- `allStrings` in `cmd/mdsmith-wasm/main.go`: it moved to
  [plan 2609201914][plan-0920], with the other WASM bridge
  helpers.
- `applyEdits`, `sortEditsByCharacterDesc`, `splitKeepCR`, and
  `joinLF` in `cmd/mdsmith/rename.go`: a separate PR redoes
  PR #839's extraction of this edit-splice code out of
  `cmd/mdsmith` and adds its own tests.
- The missing "no test by design" comments on the four
  `sessionRefactorWorkspace` pass-throughs: open PR #842's
  plan 2609131911 replaces them with one shared helper.

Two overlaps remain:

- [plan 2609271912][plan-2609271912] rewrites
  `detectRenameMode` and `detectRenameKind` around a shared
  `refactor.DetectRenameKind` and tests that new function. It
  does not add tests for the two host wrappers, so they stay
  here. If it lands first, test the wrappers as they are
  then.
- Open PR #842's plan 2609131913 names `applyEditsToFile`,
  `looksLikePath`, `firstPathish`, and `resolveWriteMode` as
  an optional stretch. This plan makes them required. Skip
  any that already have a test by name.

## Tasks

1. Add `TestGitTracked` and `TestGitMove` to
   `internal/refactor/fileop_exec_test.go`. Reuse its
   `gitInit` and `gitRun` helpers; `gitInit` already skips
   when `git` is not on `PATH`.
2. Add `TestLabelConflict`, `TestLinkRefEdits`,
   `TestRefUseEditsInBody`, `TestRefUseEdit`,
   `TestLinkTextBounds`, and `TestBodyNewlineCount` to
   `internal/refactor/rename_test.go`.
3. For the private `bodyAndFMOffset`, pick one. Either point
   its callers in `internal/refactor` at the exported
   `BodyAndFMOffset` and fold the two into one function, so
   `TestBodyAndFMOffset` covers it. Or add
   `TestBodyAndFMOffsetPrivate`, since `TestBodyAndFMOffset`
   already names the exported wrapper.
4. Add `TestApplyEditsToFile` to
   `cmd/mdsmith/move_unit_test.go`. If the edit-splice PR has
   split `applyEditsToFile` by then, test its successors
   instead.
5. Add `TestBuildWorkspace`, `TestDetectRenameMode`,
   `TestHeadingPlan`, `TestLinkRefPlan`, `TestLooksLikePath`,
   `TestFirstPathish`, and `TestResolveWriteMode` to
   `cmd/mdsmith/rename_unit_test.go`.
6. Add `TestDetectRenameKind`, `TestToRefactorPlan`,
   `TestSessionRefactorWorkspace_Resolve`, and
   `TestSession_BuildRefactorWorkspace` to
   `pkg/mdsmith/refactor_test.go`.
7. Do not delete or duplicate the scenario tests named in the
   Background section. They cover the public contract and
   stay as-is.
8. `go build ./...` and `go vet ./...` pass.

## Acceptance Criteria

- [ ] Every function listed in the Background section has a
      test carrying its own name.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[tests]: ../docs/development/architecture/tests.md
[fileop-exec]: ../internal/refactor/fileop_exec.go
[rename]: ../internal/refactor/rename.go
[perf-test]: ../internal/refactor/perf_test.go
[cmd-move]: ../cmd/mdsmith/move.go
[cmd-rename]: ../cmd/mdsmith/rename.go
[pkg-refactor]: ../pkg/mdsmith/refactor.go
[plan-0920]: 2609201914_arch-fix-missing-unit-tests-0920.md
[plan-2609271912]: 2609271912_arch-fix-shared-rename-mode-detection.md
