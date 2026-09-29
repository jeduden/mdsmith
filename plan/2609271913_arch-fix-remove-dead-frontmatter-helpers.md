---
id: 2609271913
title: >-
  Remove dead front-matter parse helpers in internal/index
status: "🔲"
model: haiku
summary: >-
  internal/index/build.go's frontMatterSymbols,
  frontMatterScalar, and frontMatterStringList are no
  longer called by production code — frontMatterAll
  replaced them — and are kept alive only so
  coverage_test.go has something to test. Flagged by the
  2026-09-27 audit as tax.
---
# Remove dead front-matter parse helpers in internal/index

## Goal

Delete the three superseded front-matter parsing helpers
and their coverage-only tests. `internal/index` should
carry no production code whose only reason to exist is a
test.

## Background

The 2026-09-27 audit (see [the audit log][audit-log]) found:

- [internal/index/build.go][build]'s `frontMatterSymbols`,
  `frontMatterScalar`, and `frontMatterStringList`
  (lines 354–487) are single-purpose predecessors to
  `frontMatterAll`, which now does the same job in one YAML
  pass. `frontMatterAll`'s own doc comment says it is
  "equivalent to calling `frontMatterSymbols` +
  `frontMatterScalar(title)` + `frontMatterStringList(kinds)`."
- The only remaining callers are in
  [internal/index/coverage_test.go][coverage-test]; the
  target of the coverage claims these functions are "kept
  ... for the targeted coverage test."
- [tests.md][tests]'s per-function unit-test rule exists to
  make sure production code is exercised, not to justify
  keeping unused production code alive as a test subject —
  this inverts that rule. It also leaves ~130 lines of dead
  code a future reader could mistake for a live path, and a
  duplicate YAML-parse implementation that can silently
  diverge from `frontMatterAll`'s behavior with nothing in
  the real build path to catch it.

## Tasks

1. Read [internal/index/build.go][build]'s
   `frontMatterSymbols`, `frontMatterScalar`, and
   `frontMatterStringList` and confirm (e.g. via
   `grep -rn` across non-test files) that `frontMatterAll`
   is the only production caller path and none of the three
   helpers is called outside `internal/index`.
2. Delete the three functions from
   [internal/index/build.go][build].
3. Delete their dedicated tests in
   [internal/index/coverage_test.go][coverage-test]; if any
   of those test cases exercise a YAML edge case (invalid
   scalar, missing key, wrong-typed value) not already
   covered by `frontMatterAll`'s own tests, port the case
   into a `frontMatterAll` test first, then delete the old
   test.
4. `go build ./...` passes.
5. `go test ./...` passes, and `internal/index`'s coverage
   does not drop below the project's coverage gate (see
   [coverage.md][coverage]).
6. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.

## Acceptance Criteria

- [ ] `frontMatterSymbols`, `frontMatterScalar`, and
      `frontMatterStringList` no longer exist in
      `internal/index`.
- [ ] `frontMatterAll`'s test coverage includes every YAML
      edge case the deleted helpers' tests exercised.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
- [ ] Codecov coverage gate still passes (no net coverage
      regression from the deletion).

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[coverage]: ../docs/development/coverage.md
[build]: ../internal/index/build.go
[coverage-test]: ../internal/index/coverage_test.go
