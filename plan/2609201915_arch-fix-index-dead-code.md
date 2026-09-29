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
  `frontMatterStringList` have no production callers.
  `grep -rn "frontMatterSymbols\|frontMatterScalar\|frontMatterStringList"
  --include="*.go" .`, filtered to non-`_test.go` results,
  turns up only the three definitions themselves and the
  doc comment on `frontMatterAll`. `internal/index` has no
  `go:build`-tagged files and no `reflect`-based dispatch
  that could hide an indirect call.
- Every production call site now goes through
  `frontMatterAll` instead.
- The doc comment on `frontMatterSymbols` admits the gap:
  "Kept exported-ish (package-private) for the targeted
  coverage test in coverage_test.go."
- `coverage_test.go` and `build_coverage_test.go` exist
  only to exercise these three otherwise-dead functions.
- A 2026-09-29 review flagged a real risk in the original
  plan text: `frontMatterAll`'s doc comment claims it is
  "equivalent to calling `frontMatterSymbols` +
  `frontMatterScalar(title)` + `frontMatterStringList(kinds)`",
  but that is only true for plain-string values.
  `frontMatterScalar` decodes each scalar into a Go type
  (`int`, `float64`, `uint64`, `bool`, `time.Time`) and
  reformats it — a bare date value like `2024-01-15` comes
  back as the RFC3339 string `2024-01-15T00:00:00Z`.
  `frontMatterAll` instead takes the YAML node's raw
  `.Value` text for `title` — the same input would come
  back as the literal `2024-01-15`. This divergence is
  latent (a `title:` value that parses as a date, int,
  float, or bool is unusual) and predates this audit, but
  deleting `frontMatterScalar`'s test
  (`TestFrontMatterScalarFormats`, which is the only place
  this reformatting is pinned) without replacing it would
  silently drop the last test that documents which behavior
  is intended.

Keeping unused code alive to preserve a coverage metric
inverts the test pyramid's intent — tests should document
real behavior, not preserve unreachable code. This is `tax`:
it costs nothing today but is pure maintenance debt.

## Tasks

1. Re-run the grep in the Background section against the
   `origin/main` this plan lands on, to confirm
   `frontMatterSymbols`, `frontMatterScalar`, and
   `frontMatterStringList` still have no production callers.
2. Before deleting anything, decide what `frontMatterAll`
   should do for a `title:` value that decodes to a
   non-string YAML scalar (int, float, bool, date): keep
   today's raw-`.Value` behavior (the common case, since a
   YAML front-matter `title` is a string in every fixture
   this repo ships), or match `frontMatterScalar`'s
   type-decode-and-reformat behavior. Whichever is chosen,
   write it down as a doc comment on `frontMatterAll` so the
   next reader isn't misled by the current "equivalent to"
   claim.
3. Port `TestFrontMatterScalarFormats`'s cases (int, float64,
   uint64 overflow, bool, ISO-8601 date, null, missing key,
   invalid YAML, no-trailing-newline) into a
   `frontMatterAll`-driven test asserting the `title` output
   the decision in task 2 calls for. Do the same for
   `TestFrontMatterStringListBranches`'s cases (missing key,
   non-list value, mixed-type list, empty, invalid YAML)
   against `frontMatterAll`'s `kinds` output, and for
   `TestFrontMatterSymbolsHandlesErrors`'s and
   `TestFrontMatterSymbolsSkipsEmptyKeys`'s cases against its
   `syms` output.
4. Only after task 3's new tests are green, delete
   `frontMatterSymbols`, `frontMatterScalar`, and
   `frontMatterStringList` from
   [internal/index/build.go][build].
5. Delete `coverage_test.go`'s and `build_coverage_test.go`'s
   tests for the three deleted functions; keep every other
   test in those files.
6. `go build ./...` passes.
7. `go test ./internal/index/...` passes.

## Acceptance Criteria

- [ ] `frontMatterSymbols`, `frontMatterScalar`, and
      `frontMatterStringList` no longer exist in
      `internal/index/build.go`.
- [ ] No test references the deleted functions.
- [ ] Every case `TestFrontMatterScalarFormats`,
      `TestFrontMatterStringListBranches`,
      `TestFrontMatterSymbolsHandlesErrors`, and
      `TestFrontMatterSymbolsSkipsEmptyKeys` covered has an
      equivalent assertion against `frontMatterAll`.
- [ ] `frontMatterAll`'s doc comment states its actual
      `title`-scalar behavior instead of the superseded
      "equivalent to" claim.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[build]: ../internal/index/build.go
