---
id: 2609201915
title: >-
  Remove internal/index dead code kept alive only for a
  coverage test
status: "🔲"
model: haiku
summary: >-
  frontMatterSymbols, frontMatterScalar, and
  frontMatterStringList in internal/index/build.go have zero
  production callers — superseded by frontMatterAll — and
  exist only so coverage_test.go/build_coverage_test.go can
  exercise them. Flagged by the 2026-09-20 audit as tax.
---
# Remove internal/index dead code kept alive only for a coverage test

## Goal

Delete unused production code instead of keeping it alive to
satisfy a coverage number.

## Background

The 2026-09-20 audit (see [the audit log][audit-log])
found dead code in [internal/index/build.go][build]:

- `frontMatterSymbols`, `frontMatterScalar`, and
  `frontMatterStringList` have no production callers. A
  repo-wide grep confirms this.
- Every production call site now goes through
  `frontMatterAll` instead.
- The doc comment on `frontMatterSymbols` admits the gap:
  "Kept exported-ish (package-private) for the targeted
  coverage test in coverage_test.go."
- `coverage_test.go` and `build_coverage_test.go` exist
  only to exercise these three otherwise-dead functions.

Keeping unused code alive to preserve a coverage metric
inverts the test pyramid's intent — tests should document
real behavior, not preserve unreachable code. This is `tax`:
it costs nothing today but is pure maintenance debt.

## Tasks

1. Confirm `frontMatterSymbols`, `frontMatterScalar`, and
   `frontMatterStringList` have no remaining production
   callers (only `frontMatterAll`-related comments reference
   them).
2. Delete the three functions from
   [internal/index/build.go][build].
3. Delete or trim `coverage_test.go` and
   `build_coverage_test.go` to remove only the tests that
   exercised the deleted functions; keep any other tests in
   those files that cover still-live code.
4. If deleting the three functions drops line coverage on
   `frontMatterAll`'s parse paths, add direct tests for
   `frontMatterAll` covering the same YAML shapes the deleted
   functions' tests exercised, so behavior coverage doesn't
   regress.
5. `go build ./...` passes.
6. `go test ./internal/index/...` passes.

## Acceptance Criteria

- [ ] `frontMatterSymbols`, `frontMatterScalar`, and
      `frontMatterStringList` no longer exist in
      `internal/index/build.go`.
- [ ] No test references the deleted functions.
- [ ] `frontMatterAll`'s behavior coverage is unchanged or
      improved.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[build]: ../internal/index/build.go
