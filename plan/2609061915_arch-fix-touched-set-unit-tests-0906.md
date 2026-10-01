---
id: 2609061915
title: >-
  Add unit tests for the untested rename/move helpers
status: "✅"
model: sonnet
summary: >-
  20 private helpers in internal/refactor and its
  cmd/mdsmith and pkg/mdsmith callers each got a unit
  test named after them. Before, only tests of the
  functions that call them reached the helpers.
---
# Add unit tests for the untested rename/move helpers

## Goal

Give each function below its own unit test by name, per
[tests.md][tests]'s "every function has a dedicated unit
test" rule.

## Background

The 2026-09-06 architecture audit of the `internal/refactor`
rename/move engine flagged these functions. That audit was in
closed PR #839 and never reached main. These functions had
no test named after them on main. One of the original 21,
`applyEditsToFile`, no longer exists, so 20 remain. They are
listed by name, not line, since the files keep moving:

- [internal/refactor/fileop_exec.go][fileop-exec] —
  `gitTracked`, `gitMove`. Only the `TestFileOpExecute_*`
  tests reach them, through `FileOp.Execute`.
- [internal/refactor/rename.go][rename] —
  `invalidLinkRefRune`, `labelConflict`, `linkRefEdits`,
  `refUseEditsInBody`, `refUseEdit`, `linkTextBounds`, and
  `bodyNewlineCount`. `TestLinkRef_InvalidRune`,
  `TestLinkRef_NewlineRuneRejected`, and
  `TestLinkRef_LabelConflict` call `LinkRef`, not the helpers.
  The `TestRefUseEditsInBodyIgnoresOtherLabels` and
  `TestRefUseEditCollapsedReferenceLabel` tests live in
  `internal/lsp` and drive a `textDocument/rename` request
  through a server harness. They are integration tests, not
  unit tests next to the source.
- [cmd/mdsmith/move.go][cmd-move] — `applyEditsToFile`
  (removed since the audit; see Task 4).
- [cmd/mdsmith/rename.go][cmd-rename] —
  `buildWorkspace`, `detectRenameMode`, `headingPlan`,
  `linkRefPlan`, `looksLikePath`, `firstPathish`,
  `resolveWriteMode`.
- [pkg/mdsmith/refactor.go][pkg-refactor] —
  `detectRenameKind`, `toRefactorPlan`,
  `(*sessionRefactorWorkspace).Resolve`, and
  `(*Session).buildRefactorWorkspace`.

Only their callers' tests reach them. Most of those are
scenario tests:

- `TestMove_*` and `TestLinkRef_*` in `internal/refactor`.
- `TestRunRename_*` and `TestRunMove_*` in `cmd/mdsmith`.
- `TestSession_Rename*` and `TestSession_Move*` in
  `pkg/mdsmith`.

A few caller tests are unit tests next to the source:

- `TestBuildRenameWorkspace_DiscoveryPaths` drove the exits
  of `buildWorkspace` through `buildRenameWorkspace`. It now
  keeps the unreadable-target exit, plus the missing-config
  and empty-workspace exits as propagation checks.
- The `TestApplyPlan_*` tests in
  [move_unit_test.go][cmd-move-test] call `buildWorkspace`
  directly, but only as setup.
- `TestComputeRenamePlan` reaches `headingPlan`. It passes
  `--as heading` every time. Only `TestRunRename_LinkRefSuccess`
  and `TestE2E_Rename_LinkRef_JSON` reach `linkRefPlan`.
- `TestWriteFilePreservingMode*` reaches `resolveWriteMode`.

None of them is a public surface by itself, so all are
`tax`, not `blocker`.

## Out of scope

Left out, with the reason:

- `bodyAndFMOffset` in [rename.go][rename]:
  `TestBodyAndFMOffset` already carries its name. It covers
  both branches through the exported `BodyAndFMOffset`, a
  one-line wrapper.
- `validRefDefMatches` in [rename.go][rename]:
  `TestValidRefDefMatchesNoRefDefsCheapNoParse` in
  [perf_test.go][perf-test] already calls it directly.
- `recomputeToken`, `encodePathToken`, `pathEdit`, and
  `countFilesWithStem` in [move.go][move]:
  [plan 2609131913][plan-2609131913] gives these four their
  own tests.
- `sortEdgesBySource` in [index.go][index]: PR #838 added
  `TestSortEdgesBySource_NoReflectSort` and
  `TestSortEdgesBySource_DeterministicAcrossInputOrders`.
- `allStrings` in [cmd/mdsmith-wasm/main.go][wasm-main]:
  [plan 2609201914][plan-0920] covers it, with the other WASM
  bridge helpers.
- `applyEdits`, `sortEditsByCharacterDesc`, `splitKeepCR`, and
  `joinLF` in [cmd/mdsmith/rename.go][cmd-rename]: main already
  has `TestApplyEdits`, `TestSortEditsByCharacterDesc_NoReflectSort`,
  and `TestSplitKeepCRAndJoinLF`. PR #856 moved the four into
  `internal/refactor` (as `refactor.ApplyEdits`) together with
  those tests.
- The "no test by design" comments that the four one-line
  `sessionRefactorWorkspace` pass-throughs in
  [refactor.go][pkg-refactor] lack:
  [plan 2609131911][plan-2609131911] replaces them with one
  shared implementation. If that plan is dropped, add the four
  comments here, matching the ones on `cliRenameWorkspace` in
  [cmd/mdsmith/rename.go][cmd-rename].

Two overlaps remain:

- [plan 2609271912][plan-2609271912] rewrites
  `detectRenameMode` and `detectRenameKind` around a shared
  `refactor.DetectRenameKind` and tests that new function. It
  does not add tests for the two host wrappers, so they stay
  here. If it lands first, test the wrappers as they are
  then.
- [Plan 2609131913][plan-2609131913] names `applyEditsToFile`,
  `looksLikePath`, `firstPathish`, and `resolveWriteMode` as
  an optional stretch. Task 1 handles any test it adds.

## Tasks

1. Before writing each test, search the package's test files
   for a test already named after the function, such as one
   [plan 2609131913][plan-2609131913] may add. If one exists,
   skip that function.
   Do not add a second test with the same name; Go rejects a
   package that declares a function twice.
2. Add `TestGitTracked` and `TestGitMove` to
   [fileop_exec_test.go][fileop-exec-test]. Reuse its
   `gitInit` and `gitRun` helpers; `gitInit` already skips
   when `git` is not on `PATH`.
3. Add `TestInvalidLinkRefRune`, `TestLabelConflict`,
   `TestLinkRefEdits`, `TestRefUseEditsInBody`,
   `TestRefUseEdit`, `TestLinkTextBounds`, and
   `TestBodyNewlineCount` to [rename_test.go][rename-test].
4. Skipped. `applyEditsToFile` no longer exists on main: PR #859
   replaced it with an all-or-nothing plan applier whose
   functions carry their own tests.
5. Add `TestBuildWorkspace`, `TestDetectRenameMode`,
   `TestHeadingPlan`, `TestLinkRefPlan`, `TestLooksLikePath`,
   `TestFirstPathish`, and `TestResolveWriteMode` to
   [rename_unit_test.go][cmd-rename-test].
   `TestBuildWorkspace` calls `buildWorkspace` directly for
   each exit: missing config, empty workspace, bad
   max-input-size, and success. Move the "bad max-input-size
   exits 2" subtest out of
   `TestBuildRenameWorkspace_DiscoveryPaths` into it, rather
   than assert the same exit twice. Keep "unreadable target
   exits 2" in the `buildRenameWorkspace` test. That test
   still needs `buildWorkspace` failures to cover its early
   return, so "missing config exits 2" and "empty workspace
   exits 1" are asserted in both tests on purpose: the
   second proves the exit code propagates unchanged.
6. Add `TestDetectRenameKind`, `TestToRefactorPlan`,
   `TestSessionRefactorWorkspace_Resolve`, and
   `TestSession_BuildRefactorWorkspace` to
   [refactor_test.go][pkg-refactor-test].
7. Keep the caller tests named in the Background section. They
   cover the public contract. Task 5's subtest move is the one
   change to them.
8. Fix the production bugs the new tests exposed:

  - `linkTextBounds` anchored on the inner text nodes, so a
     use like `[**bold**][docs]`, ``[`code`][docs]``, or
     `[][docs]` was dropped. It now anchors on the link's
     recorded `[` and scans to the balancing `]`, skipping
     escapes and code spans.
  - `refUseEditsInBody` skipped image references such as
     `![alt][docs]`. It now rewrites them too.
  - `invalidLinkRefRune` accepted a label that ends in an
     unescaped backslash, which escapes the def's closing `]`.
     It now rejects one.
  - The `buildRenameWorkspace` comment said an empty
     workspace returns 0. It returns 1.

9. `go build ./...` and `go vet ./...` pass.

## Acceptance Criteria

- [x] Every function listed in the Background section that
      still exists has a test carrying its own name, added
      here or earlier by another PR.
- [x] No test name is declared twice in a package.
- [x] Reference uses with inline markup, empty text, or an
      image are rewritten, and a label ending in an unescaped
      backslash is rejected.
- [x] `go test ./...` is green.
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [x] `mdsmith check .` is green.

[tests]: ../docs/development/architecture/tests.md
[fileop-exec]: ../internal/refactor/fileop_exec.go
[fileop-exec-test]: ../internal/refactor/fileop_exec_test.go
[rename]: ../internal/refactor/rename.go
[rename-test]: ../internal/refactor/rename_test.go
[perf-test]: ../internal/refactor/perf_test.go
[move]: ../internal/refactor/move.go
[index]: ../internal/index/index.go
[wasm-main]: ../cmd/mdsmith-wasm/main.go
[cmd-move]: ../cmd/mdsmith/move.go
[cmd-move-test]: ../cmd/mdsmith/move_unit_test.go
[cmd-rename]: ../cmd/mdsmith/rename.go
[cmd-rename-test]: ../cmd/mdsmith/rename_unit_test.go
[pkg-refactor]: ../pkg/mdsmith/refactor.go
[pkg-refactor-test]: ../pkg/mdsmith/refactor_test.go
[plan-0920]: 2609201914_arch-fix-missing-unit-tests-0920.md
[plan-2609131911]: 2609131911_arch-fix-refactor-workspace-duplication.md
[plan-2609131913]: 2609131913_arch-fix-move-helper-unit-tests.md
[plan-2609271912]: 2609271912_arch-fix-shared-rename-mode-detection.md
