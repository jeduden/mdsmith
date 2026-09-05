---
id: 2608161914
title: >-
  Add dedicated unit tests for the 2026-08-16 touched-set
  tax findings
status: "✅"
model: haiku
summary: >-
  Six new functions across cmd/mdsmith, pkg/mdsmith,
  internal/rules/duplicatedcontent, internal/rules/astutil,
  internal/bytelimit, and pkg/markdown/flavor landed with
  only behavior-level coverage, not a dedicated unit test
  by name. Flagged by the 2026-08-16 audit as tax.
---
# Add dedicated unit tests for the 2026-08-16 touched-set tax findings

## Goal

Give each function below its own unit test by name. Match
the `TestFoo`/`TestReceiver_Foo` convention [tests.md][tests]
requires — the one every untouched sibling package already
follows.

## Background

The 2026-08-16 audit (see [the audit log][audit-log]) swept
every file touched since the prior checkpoint. It found six
functions with no test carrying their own name. Each is
exercised only indirectly through a caller's scenario test:

- [cmd/mdsmith/init.go][init]:125-244 — `runInitConfig`,
  `applyAPMPosture`, `apmIgnoreGlobs`, `apmPostureBlock`, and
  `printAPMMergeHint`. Covered only via the end-to-end
  `TestRunInit_APM_*` family in
  [init_unit_test.go][init-test]:304-476.
- [internal/rules/duplicatedcontent/rule.go][dup]:411-470 —
  `corpusFiles` and `corpusFilesKey` (the RunCache-memoized
  corpus walk). Covered only via
  `TestCheck_HonorsIncludeExcludePattern_WithRunCache` in
  [rule_test.go][dup-test]:192.
- [pkg/mdsmith/workspace.go][workspace]:319-372 —
  `buildDirIndex` and `addDirEntry` (the memFS directory
  index). Covered only via
  `TestMemFSDirEntriesIgnoresEmptySegment` and
  `TestMemFSDirEntriesIgnoresTrailingSlashKey` in
  [workspace_test.go][workspace-test]:301,333.
- [internal/bytelimit/bytelimit.go][bytelimit]:31-34 —
  `FileTooLargeError`, an exported cross-package helper.
  Covered only transitively through `TestReadFileLimited_OverLimit`
  and callers' own error-message assertions.
- [internal/rules/astutil/astutil.go][astutil]:114-133 —
  `buildHeadingNodes` and `sortSectionHeadings`. Covered only
  via `TestCollectHeadingNodes_*` in
  [astutil_test.go][astutil-test]:708-739.
- [pkg/markdown/flavor/detect.go][detect]:461-467 —
  `newlineSearch`, the binary-search helper backing the new
  `lineIndex`. Covered only via `TestLineIndex_MatchesLineCol`
  in [lineindex_test.go][lineindex-test]:14.

All six are `tax`: none sit on a public surface by
themselves (their callers' `rule.Rule`/`Session`/CLI-entry
methods already have tests).

## Tasks

1. Add `TestRunInitConfig`, `TestApplyAPMPosture`,
   `TestApmIgnoreGlobs`, `TestApmPostureBlock`, and
   `TestPrintAPMMergeHint` to
   [init_unit_test.go][init-test].
2. Add `TestCorpusFiles` and `TestCorpusFilesKey` to
   [duplicatedcontent/rule_test.go][dup-test].
3. Add `TestBuildDirIndex` and `TestAddDirEntry` to
   [pkg/mdsmith/workspace_test.go][workspace-test].
4. Add `TestFileTooLargeError` to a `bytelimit_test.go` file
   next to [bytelimit.go][bytelimit].
5. Add `TestBuildHeadingNodes` and `TestSortSectionHeadings`
   to [astutil/astutil_test.go][astutil-test].
6. Add `TestNewlineSearch` to
   [pkg/markdown/flavor/lineindex_test.go][lineindex-test]
   (or a sibling `detect_test.go`, matching the existing
   `internal/lint` precedent for the same helper).
7. Do not delete or duplicate the existing behavior-level
   tests named above. They cover the public contract and
   stay as-is.
8. `go build ./...` passes.
9. `go test ./cmd/mdsmith/... ./internal/rules/duplicatedcontent/...
   ./pkg/mdsmith/... ./internal/bytelimit/...
   ./internal/rules/astutil/... ./pkg/markdown/flavor/...`
   passes.

## Acceptance Criteria

- [x] Every function named in the Background section has a
      test carrying its own name (or a named subtest under
      one parent test, per tests.md's "sub-behaviours"
      allowance).
- [x] `go test ./...` is green.
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [x] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[init]: ../cmd/mdsmith/init.go
[init-test]: ../cmd/mdsmith/init_unit_test.go
[dup]: ../internal/rules/duplicatedcontent/rule.go
[dup-test]: ../internal/rules/duplicatedcontent/rule_test.go
[workspace]: ../pkg/mdsmith/workspace.go
[workspace-test]: ../pkg/mdsmith/workspace_test.go
[bytelimit]: ../internal/bytelimit/bytelimit.go
[astutil]: ../internal/rules/astutil/astutil.go
[astutil-test]: ../internal/rules/astutil/astutil_test.go
[detect]: ../pkg/markdown/flavor/detect.go
[lineindex-test]: ../pkg/markdown/flavor/lineindex_test.go
