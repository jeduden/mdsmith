---
id: 2609061915
title: >-
  Add unit tests for the 2026-09-06 rename/move tax findings
status: "🔲"
model: haiku
summary: >-
  The rename/move refactor engine and its CLI/WASM callers
  landed with many private helpers covered only through
  their callers' scenario tests, not a dedicated unit test
  by name. Flagged by the 2026-09-06 audit as tax.
---
# Add unit tests for the 2026-09-06 rename/move tax findings

## Goal

Give each function below its own unit test by name, per
[tests.md][tests]'s "every function has a dedicated unit
test" rule.

## Background

The 2026-09-06 audit (see [the audit log][audit-log]) swept
the `internal/refactor` engine (new this cycle) and its
`cmd/mdsmith` / `pkg/mdsmith` / `cmd/mdsmith-wasm` callers.
None of the functions below sit on a public surface by
themselves. Each is exercised only indirectly, through a
caller's end-to-end scenario test (`TestMove_*`,
`TestLinkRef_*`, `TestRunRename_*`, the Obsidian WASM smoke
test). All are `tax`, not `blocker`.

- [internal/refactor/fileop_exec.go][fileop-exec]:54,65 —
  `gitTracked`, `gitMove`.
- [internal/refactor/move.go][move]:129,146,268,507 —
  `recomputeToken`, `encodePathToken`, `pathEdit`,
  `countFilesWithStem`.
- [internal/refactor/rename.go][rename]:169,198,254,300,329,393,472,508 —
  `labelConflict`, `validRefDefMatches`, `linkRefEdits`,
  `refUseEditsInBody`, `refUseEdit`, `linkTextBounds`,
  `bodyNewlineCount`, `bodyAndFMOffset`.
- [cmd/mdsmith/move.go][cmd-move]:179 — `applyEditsToFile`.
- [cmd/mdsmith/rename.go][cmd-rename]:215,272,300,320,337,346,356 —
  `buildWorkspace`, `detectRenameMode`, `headingPlan`,
  `linkRefPlan`, `looksLikePath`, `firstPathish`,
  `resolveWriteMode`.
- [internal/index/index.go][index]:557 — `sortEdgesBySource`
  (the wikilink-edge sort added this cycle).
- [pkg/mdsmith/refactor.go][pkg-refactor]:104,122,168,184 —
  `detectRenameKind`, `toRefactorPlan`,
  `(*sessionRefactorWorkspace).Resolve`,
  `(*Session).buildRefactorWorkspace`.
- [cmd/mdsmith-wasm/main.go][wasm-main]:235 — `allStrings`
  (the JS-arg-type guard the new `rename`/`move` WASM
  methods added this cycle both call).

Also a doc nit, not a task here.

Four trivial `sessionRefactorWorkspace` pass-throughs in
[pkg/mdsmith/refactor.go][pkg-refactor] (`IncomingAnchorEdges`,
`IncomingPathEdges`, `IncomingWikilinkEdges`, `Files`) miss
the "no test by design" comment.

Their twins in [internal/lsp/rename.go][lsp-rename] and
[cmd/mdsmith/rename.go][cmd-rename] already carry it. Add
the comment next time this file is touched.

## Tasks

1. Add `TestGitTracked` and `TestGitMove` to a
   `fileop_exec_test.go` sibling (extend the existing one) in
   `internal/refactor`.
2. Add `TestRecomputeToken`, `TestEncodePathToken`,
   `TestPathEdit`, and `TestCountFilesWithStem` to
   `move_test.go` / `move_coverage_test.go`.
3. Add `TestLabelConflict`, `TestValidRefDefMatches`,
   `TestLinkRefEdits`, `TestRefUseEditsInBody`,
   `TestRefUseEdit`, `TestLinkTextBounds`,
   `TestBodyNewlineCount`, and `TestBodyAndFMOffset` to
   `rename_test.go`.
4. Add `TestApplyEditsToFile` to `cmd/mdsmith`'s
   `move_unit_test.go`.
5. Add `TestBuildWorkspace`, `TestDetectRenameMode`,
   `TestHeadingPlan`, `TestLinkRefPlan`, `TestLooksLikePath`,
   `TestFirstPathish`, and `TestResolveWriteMode` to
   `cmd/mdsmith`'s `rename_unit_test.go`.
6. Add `TestSortEdgesBySource` to `internal/index`.
7. Add `TestDetectRenameKind`, `TestToRefactorPlan`,
   `TestSessionRefactorWorkspace_Resolve`, and
   `TestSession_BuildRefactorWorkspace` to
   `pkg/mdsmith/refactor_test.go`.
8. Add `TestAllStrings` to `cmd/mdsmith-wasm`'s
   `methods_test.go` (or a new `main_test.go` restricted to
   the `js && wasm` build tag, matching how the package
   already tests wasm-only helpers).
9. Do not delete or duplicate the existing scenario tests
   named in the Background section — they cover the public
   contract and stay as-is.
10. `go build ./...` and `go vet ./...` pass.
11. `GOOS=js GOARCH=wasm go build ./cmd/mdsmith-wasm/...`
    still builds (task 8 only, since `allStrings` lives in a
    `js && wasm`-tagged file).

## Acceptance Criteria

- [ ] Every function named in the Background section has a
      test carrying its own name.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[fileop-exec]: ../internal/refactor/fileop_exec.go
[move]: ../internal/refactor/move.go
[rename]: ../internal/refactor/rename.go
[cmd-move]: ../cmd/mdsmith/move.go
[cmd-rename]: ../cmd/mdsmith/rename.go
[index]: ../internal/index/index.go
[pkg-refactor]: ../pkg/mdsmith/refactor.go
[wasm-main]: ../cmd/mdsmith-wasm/main.go
[lsp-rename]: ../internal/lsp/rename.go
