---
id: 2606071931
title: Unit tests for include-rule private validation helpers
status: "✅"
summary: >-
  Add dedicated unit tests for the four private helpers
  extracted from validateIncludeDirective in plan 232/233.
model: haiku
depends-on: []
---
# Unit tests for include-rule private validation helpers

## Goal

Add a named `Test<Function>` for each of the four private
helpers extracted from `validateIncludeDirective` in plans
232 and 233.

## Background

Commits `09403b4` (plan 232) and `74c72dc` (plan 233)
extracted four private helpers:

- `validateHeadingOffset` — integer −6 to 6, conflicts with
  `heading-level`.
- `validateHeadingLevel` — numeric h1–h6 or named level.
- `validateExtractParam` — extract parameter validation.
- `findParentHeadingLevel` — AST walk for nearest heading.

Each commit added AST-transform tests only.
The validation wrappers have no direct unit test.
Integration and fixture tests cover them indirectly.
No `Test<Function>` symbol names them.

Severity: tax (private; covered indirectly, but no red/green
entry point for future refactors).

## Tasks

1. Add `TestValidateHeadingOffset` in
   `internal/rules/include/headings_test.go`:

  - missing param, invalid integer, out-of-range, valid,
     conflict with `heading-level`.

2. Add `TestValidateHeadingLevel`:

  - missing param, invalid string, valid numeric/named.

3. Add `TestValidateExtractParam` covering its branches.
4. Add `TestFindParentHeadingLevel`:

  - no heading above marker; heading found at depth.

5. Run `go test ./internal/rules/include/... -v` and confirm
   the new `Test*` names appear.

## Acceptance Criteria

- [x] `TestValidateHeadingOffset` covers five cases.
- [x] `TestValidateHeadingLevel` covers three cases.
- [x] `TestValidateExtractParam` has branch coverage.
- [x] `TestFindParentHeadingLevel` covers both cases.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
