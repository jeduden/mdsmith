---
id: 2606162214
title: >-
  Add named unit tests for unexported
  helpers in new lazy-parse packages
status: "🔲"
model: sonnet
depends-on: []
summary: >-
  Several unexported helper functions
  introduced by the lazy-parse series
  (internal/rule/walk.go,
  internal/checker/checker.go,
  internal/lint/linkrefscan.go,
  internal/lint/codespans.go) lack a
  dedicated unit test by name, violating
  the "every function ships with a test"
  rule.
---
# Add named unit tests for helper functions

## Context

Closes audit entry from the
[2026-06-16 audit][audit]:
"tax — missing named unit tests for
unexported helpers in new lazy-parse
packages."

[audit]: ../docs/development/architecture-audit.md

## Goal

One `TestFoo` per listed function, directly
exercising it. Sub-behaviours go under
`t.Run`. Go arch doc §"Go-specific bindings"
names this pattern.

## Tasks

1. Add to `internal/rule/walk_test.go`:

  - `TestBlockKindInSet`

2. Add to `internal/checker/checker_test.go`:

  - `TestClassifyRules`
  - `TestClassifySlot`
  - `TestBuildKindTable`
  - `TestBlockCheckerReactsTo`
  - `TestRunNonNodeCheckers`
  - `TestRunNodeCheckers`
  - `TestRunBlockCheckers`

3. Add to `internal/lint/linkrefscan_test.go`:

  - `TestScanNeedsFallback`
  - `TestParagraphHeadMayDefine`
  - `TestBuildLineSegments`

4. Add to `internal/lint/codespans_test.go`:

  - `TestBytesView`

5. Run `go test ./internal/rule/...
   ./internal/checker/... ./internal/lint/...`
   after each batch to stay green.

## Acceptance Criteria

- [ ] `TestBlockKindInSet` in
      `internal/rule/walk_test.go`.
- [ ] `TestClassifyRules`,
      `TestClassifySlot`,
      `TestBuildKindTable`,
      `TestBlockCheckerReactsTo`,
      `TestRunNonNodeCheckers`,
      `TestRunNodeCheckers`,
      `TestRunBlockCheckers` in
      `internal/checker/checker_test.go`.
- [ ] `TestScanNeedsFallback`,
      `TestParagraphHeadMayDefine`,
      `TestBuildLineSegments` in
      `internal/lint/linkrefscan_test.go`.
- [ ] `TestBytesView` in
      `internal/lint/codespans_test.go`.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run`
      reports no issues.
