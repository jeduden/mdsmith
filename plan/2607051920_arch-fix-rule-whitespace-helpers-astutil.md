---
id: 2607051920
title: >-
  Consolidate duplicated leading-space/blank-line rule
  helpers into internal/rules/astutil
status: "✅"
model: sonnet
summary: >-
  countLeadingSpaces and isBlank/isBlankLine are
  duplicated, byte-identical, across listindent,
  orderedlistnumbering, and noreferencestyle. Export one
  copy from astutil. Flagged by the 2026-07-05 audit.
---
# Consolidate rule whitespace helpers into astutil

## Goal

Replace the duplicated `countLeadingSpaces` and
`isBlank`/`isBlankLine` helpers in `listindent`,
`orderedlistnumbering`, and `noreferencestyle` with one
shared implementation exported from
`internal/rules/astutil`.

## Background

The 2026-07-05 audit found this duplication:

- Commit `39a21e63` replaced hand-rolled byte
  loops with `bytes` package calls.
- It rewrote the same two helpers identically in
  three rule packages, shown below.

- `countLeadingSpaces(line []byte) int` — identical body
  `len(line) - len(bytes.TrimLeft(line, " "))` in
  `internal/rules/listindent/rule.go:184` and
  `internal/rules/orderedlistnumbering/rule.go:378`.
- `isBlank`/`isBlankLine(line []byte) bool` — identical
  body `len(bytes.TrimLeft(line, " \t")) == 0` in
  `internal/rules/orderedlistnumbering/rule.go:290` and
  `internal/rules/noreferencestyle/rule.go:500`.

`internal/rules/astutil` is the doc-sanctioned shared
home for exactly this kind of helper (Go architecture
doc, "rule-to-rule imports" section). A rule package
must not import another rule package, but every rule
package may import `astutil`. `listscan` (touched by
the same perf commit) is excluded: its
`openingFenceRel` isn't duplicated elsewhere, so it
stays as-is.

## Tasks

1. Add `CountLeadingSpaces(line []byte, cutset string) int`
   and `IsBlank(line []byte, cutset string) bool` (or two
   pairs, one fixed to `" "` and one to `" \t"`, matching
   the two call shapes below) to
   `internal/rules/astutil/astutil.go`.
2. Add dedicated tests in
   `internal/rules/astutil/astutil_test.go`.
3. In `internal/rules/listindent/rule.go`, delete
   `countLeadingSpaces`; call the astutil helper.
4. In `internal/rules/orderedlistnumbering/rule.go`,
   delete `countLeadingSpaces` and `isBlank`; call the
   astutil helpers.
5. In `internal/rules/noreferencestyle/rule.go`, delete
   `isBlankLine`; call the astutil helper.
6. `go build ./...` passes.
7. `go test ./internal/rules/listindent/...
   ./internal/rules/orderedlistnumbering/...
   ./internal/rules/noreferencestyle/...
   ./internal/rules/astutil/...` passes.
8. Confirm the allocation-budget test
   (`internal/integration/alloc_budget_test.go`) still
   passes for all three rules.

## Acceptance Criteria

- [x] `internal/rules/astutil` exports the shared
      leading-space and blank-line helpers, each with a
      dedicated test.
- [x] `listindent`, `orderedlistnumbering`, and
      `noreferencestyle` no longer define their own
      copies.
- [x] `go test ./...` is green.
- [x] `mdsmith check .` is green.
