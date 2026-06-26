---
id: 2606260211
title: >-
  Add dedicated unit tests for layer0_html.go
  helpers
status: "✅"
summary: >-
  Seven unexported helpers in
  internal/lint/layer0_html.go lack dedicated
  unit tests. The 2026-06-26 audit caught them
  when the perf commit touched the file.
model: sonnet
---
# Add unit tests for layer0_html helpers

## Context

The 2026-06-26 audit (range: `3d35b77..fe7141b`)
found `internal/lint/layer0_html.go` in the
touched set via the map→struct perf commit.
Seven unexported helpers lack dedicated unit
tests. Tests doc §"every function by name"
requires `TestFunctionName` for each.

`layer0_html_test.go` currently only tests
`tagInAllowedSet` (two test functions).

## Goal

Add dedicated unit tests for the seven
unexported helpers in
`internal/lint/layer0_html.go` that lack
`TestFunctionName`-style coverage.

## Tasks

1. Add `TestOpenHTMLBlock`. Check each opener type:
   type-1 raw tags, type-2 comment, type-3 PI,
   type-4 decl, type-5 CDATA, type-6 block tag,
   type-7 tag. Check that `inParagraph=true`
   blocks type-7. Check non-HTML lines return
   `htmlNone`.

2. Add `TestTagName_LowerInto` — lower-cases
   ASCII upper, preserves lowercase, truncates
   at 32 bytes, returns the correct slice.

3. Add `TestType7TagIsRawText` — returns true
   for script/style/pre/textarea (any case),
   false for div and span.

4. Add `TestType7TagBytes` — extracts the tag
   name bytes from a type-7 line, handles
   closing-tag slash, handles leading spaces.

5. Add `TestIsTagByte` — true for letters,
   digits, hyphen; false for `/`, `>`, `!`.

6. Add `TestHTMLBlockCloses`. Type-1 closes on
   `</script>` (case-insensitive). Type-2 on
   `-->`, type-3 on `?>`, type-4 on `>`,
   type-5 on `]]>`. Types 6–7 return false
   (blank-line close handled by caller).

7. Add `TestScanner_TryHTMLBlock` — build a
   minimal `scanner` with `lines` set, call
   `tryHTMLBlock`; verify the span added and
   cursor advance for a comment block, a type-6
   block, and a non-HTML line. The `scanner`
   type and `addSpan` are in the same package
   so the test file has access.

## Acceptance Criteria

- [x] `go test ./internal/lint/...` green.
- [x] `go vet ./...` green.
- [x] Each of the seven functions has a test
      named exactly per the Go binding in
      Tests doc §"Go-specific bindings".
- [x] No new production code changed (tests
      only).
- [x] `mdsmith check .` passes (0 failures).
