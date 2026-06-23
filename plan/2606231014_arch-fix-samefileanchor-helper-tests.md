---
id: 2606231014
title: Add dedicated unit tests for samefileanchor helper functions
status: "✅"
model: sonnet
summary: >-
  internal/rules/samefileanchor/rule.go has 12
  unexported helper functions covered only through
  the higher-level TestRule_* suite. Add a named
  unit test for each so the audit policy is met.
---
# Add dedicated unit tests for samefileanchor helpers

## Goal

`internal/rules/samefileanchor/rule.go` (MDS070,
plan 2606210840 / PR #675) has 12 unexported
helpers. They are covered only through the 37-case
`TestRule_*` suite. The audit policy requires a
named test for each function.

## Background

Functions in `internal/rules/samefileanchor/
rule.go` with no dedicated test as of commit
1599c9f:

- `sameFileFragment`
- `collectSlugs`
- `collectSlugsAST`
- `collectSlugsNode`
- `appendHeadingText`
- `appendHeadingTextRaw`
- `skipLinkDest`
- `skipRefLabel`
- `insertDisambiguated`
- `collectSlugsLayer0`
- `atxHeadingText`
- `appendSlug`

## Tasks

1. For each function above, add at least one
   test to `internal/rules/samefileanchor/
   helpers_test.go` (package `samefileanchor`,
   not the external `_test` package) named
   `TestFunctionName` (or `TestFunctionName_Variant`).
2. Drive each helper directly — do not build a
   full `lint.File` unless the helper signature
   requires it.
3. Run `go test ./internal/rules/samefileanchor/...`
   to confirm all pass.
4. Run `go vet ./internal/rules/samefileanchor/...`
   and golangci-lint — both must pass.

## Acceptance Criteria

- [x] `helpers_test.go` (package `samefileanchor`)
      contains named tests for all 12 helpers listed above.
- [x] `go test ./internal/rules/samefileanchor/...`
      is green.
- [x] No new golangci-lint warnings.
