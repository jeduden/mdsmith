---
id: 2606231013
title: Add dedicated unit tests for inline_scan.go helpers
status: "✅"
model: sonnet
summary: >-
  internal/lint/inline_scan.go has 13 unexported
  helper functions exercised only via the
  higher-level TestScanInlineRun_* suite. Add a
  named unit test for each so the audit policy
  (every function has a TestFoo / TestReceiver_Foo
  by name) is satisfied.
---
# Add dedicated unit tests for inline_scan.go helpers

## Goal

`internal/lint/inline_scan.go` (plan 2606141904 /
PR #632) has 13 unexported helper functions. They
are tested only through the 55 higher-level
`TestScanInlineRun_*` behavioral tests. The audit
policy requires a named test for every function.
This plan adds those tests.

## Background

The audit policy (tests.md) requires a named
test for every production function. Use `TestFoo`
for a package function `Foo`. Use
`TestReceiver_Foo` for a method on `Receiver`.

Functions in `inline_scan.go` with no dedicated
test as of commit 1599c9f:

- `scanRunEligible`
- `mergeAppendText`
- `finalAppendText`
- `scanParagraphInlines`
- `applyCodeSpan`
- `applyAutolink`
- `applyBang`
- `applyLink`
- `scanCodeSpan` (covered by `TestScanInlineRun_*`
  but not by `TestScanCodeSpan`)
- `scanLinkOrImage`
- `scanLinkParens`
- `skipSpacesAt`
- `isSpaceOrNewlineByte`

## Tasks

1. For each function above, add at least one
   test function named `TestFunctionName` (or
   `TestFunctionName_Variant` for multiple cases)
   to `internal/lint/inline_scan_test.go`.
2. Each test must be a standalone unit test:
   drive the helper directly, not via
   `scanInlineRun`.
3. Keep each test function short (≤ 15 lines).
   Use table-driven style only when three or
   more cases differ in one dimension.
4. `isSpaceOrNewlineByte` has two boolean branches
   (`c == ' '` and `c == '\n'`), so the trivial-
   accessor exemption does not apply. Add
   `TestIsSpaceOrNewlineByte` instead.
5. Run `go test ./internal/lint/... -run TestScan`
   to confirm all pass.
6. Run `go vet ./internal/lint/...` and
   `go tool -modfile=tools/go.mod golangci-lint run
   ./internal/lint/...` — both must pass.

## Acceptance Criteria

- [x] `internal/lint/inline_scan_test.go` contains
      `TestScanRunEligible`, `TestMergeAppendText`,
      `TestFinalAppendText`, `TestScanParagraphInlines`,
      `TestApplyCodeSpan`, `TestApplyAutolink`,
      `TestApplyBang`, `TestApplyLink`,
      `TestScanCodeSpan`, `TestScanLinkOrImage`,
      `TestScanLinkParens`, `TestSkipSpacesAt`,
      `TestIsSpaceOrNewlineByte` (13 new test functions).
- [x] `isSpaceOrNewlineByte` has a dedicated test
      (`TestIsSpaceOrNewlineByte`); the predicate has
      two branches so the trivial-accessor exemption
      does not apply.
- [x] `go test ./internal/lint/...` is green.
- [x] No new golangci-lint warnings.
