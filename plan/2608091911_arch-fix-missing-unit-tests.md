---
id: 2608091911
title: >-
  Add dedicated unit tests for occurrence and
  slidevstructure private helpers
status: "✅"
model: haiku
summary: >-
  internal/rules/occurrence and
  internal/rules/slidevstructure have 18 and 11
  non-trivial private helpers respectively with no
  dedicated unit test by name, breaking from the
  project's own TestFoo/TestReceiver_Foo convention.
  Flagged by the 2026-08-09 audit as tax.
---
# Add dedicated unit tests for occurrence and slidevstructure private helpers

## Goal

Give each non-trivial private helper in the `occurrence` and
`slidevstructure` rule packages its own unit test by name.
Match the convention every other rule package in the tree
already follows.

## Background

The 2026-08-09 audit (see [the audit log][audit-log]) found
two rule packages that ship only behavior-level tests. No
test carries an individual helper's own name:

- [occurrence/rule.go][occurrence] — 18 helpers with no
  dedicated test: `countCombinedInRange`,
  `countPatternInRange`, `countCombined`, `searchText`,
  `countToken`, `countPattern`, `diagEach`, `diagCombined`,
  `boundMessage`, `applyScope`, `applyTokens`,
  `extractPattern`, `applyMin`, `applyMax`, `applyCount`,
  `applyCaseSensitive`, `finalizeSettings`,
  `compileAndSetPattern`. All are exercised indirectly
  through 47 `TestCheck_*`/`TestApplySettings_*` cases.
- [slidevstructure/rule.go][slidev] — 11 helpers with no
  dedicated test: `checkSlide`, `slideAnchor`,
  `checkUnknownLayout`, `checkMissingSlots`,
  `checkOrphanedSlots`, `checkRequiredField`,
  `checkUnknownFMKeys`, `isCustomLayout`, `diag`,
  `sortedKeys`, `nearest`. All are exercised indirectly
  through 35 `Test*` cases.

[tests.md][tests] requires a dedicated unit test by name
for every production function, following the
`TestFoo`/`TestReceiver_Foo` pattern.

[audit-checklist.md][audit-checklist] confirms this is the
live convention, not aspirational text. The untouched
sibling rule `paragraphstructure` follows it for every one
of its own private helpers.

Both findings are `tax`: neither package's helpers are
themselves a public surface (a `rule.Rule` method, LSP
handler, or CLI entry). `Check` and `ApplySettings`, the
actual `rule.Rule` methods, already have tests.

## Tasks

1. For each of the 18 `occurrence` helpers above, add a
   `TestCountCombinedInRange`-style unit test — or fold
   related cases into one parent test with `t.Run`
   subtests per tests.md's "sub-behaviours" allowance — in
   [occurrence/rule_test.go][occurrence-test].
2. For each of the 11 `slidevstructure` helpers above, add
   a matching dedicated test in
   [slidevstructure/rule_test.go][slidev-test].
3. Do not delete or duplicate the existing behavior-level
   `TestCheck_*`/`TestApplySettings_*` tests. They cover
   the public `rule.Rule` contract and stay as-is.
4. `go build ./...` passes.
5. `go test ./internal/rules/occurrence/...
   ./internal/rules/slidevstructure/...` passes.

## Acceptance Criteria

- [x] Every helper named in the Background section has a
      test carrying its own name (or is covered by a named
      subtest under one parent test) in the package's
      `rule_test.go`.
- [x] `go test ./...` is green.
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [x] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[occurrence]: ../internal/rules/occurrence/rule.go
[slidev]: ../internal/rules/slidevstructure/rule.go
[tests]: ../docs/development/architecture/tests.md
[audit-checklist]: ../docs/development/architecture/audit-checklist.md
[occurrence-test]: ../internal/rules/occurrence/rule_test.go
[slidev-test]: ../internal/rules/slidevstructure/rule_test.go
