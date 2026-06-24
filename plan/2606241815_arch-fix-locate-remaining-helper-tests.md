---
id: 2606241815
title: >-
  Add unit tests for three remaining unexported
  helpers in internal/index/locate.go
status: "✅"
model: sonnet
summary: >-
  Add TestPiContainsLine, TestRefDefOnLine, and
  TestLocateInFrontMatter in
  internal/index/locate_test.go — completing the
  helper coverage started by plan 2606240211.
---
# Add unit tests for three remaining locate helpers

## Context

The 2026-06-24 audit (range: 09f22d3..3d35b77)
covered
[internal/index/locate.go](../internal/index/locate.go).
Plan 2606240211 added tests for 12 of 15 helpers.
Three were omitted:

- `piContainsLine(source []byte, pi, line int) bool`
  — reports whether a processing-instruction node
  spans the given line. Exercised at the `Locate`
  level via `TestLocatePIContainsLineMultiline` but
  has no test named after the function.
- `refDefOnLine(body []byte, lines [][]byte, line,
  col int) (LocateResult, bool)` — returns a
  reference-definition locate result when the cursor
  sits on a ref-def line. Covered via
  `TestLocateRefDef` but no `TestRefDefOnLine`.
- `locateInFrontMatter(fm []byte, line, col int)
  LocateResult` — identifies the YAML front-matter
  key or list-item under the cursor. Covered via
  `TestLocateFrontMatterKey` etc. but no
  `TestLocateInFrontMatter`.

## Goal

Every function in `internal/index/locate.go`
needs a unit test by name. See Tests doc
§"every function has a dedicated unit test".

## Tasks

1. Add `TestPiContainsLine` in
   `internal/index/locate_test.go`. Cover: line
   within the PI range returns true; line before
   the opening returns false; line after the closing
   returns false; single-line PI.
2. Add `TestRefDefOnLine` covering: cursor on the
   label segment of a ref-def returns the expected
   `LocateResult`; line that has no ref-def returns
   `false`; line number out of range returns `false`.
3. Add `TestLocateInFrontMatter` covering: key
   resolution at the cursor, list-item resolution,
   empty front matter returns a zero result.
4. While editing the file, add "// no test by design"
   to `isGlobPattern` (noted in the 2026-06-24 and
   09f22d3..3d35b77 audits).
5. Run `go test ./internal/index/...` — green.
6. Run `mdsmith check .` — 0 failures.

## Acceptance Criteria

- [x] `TestPiContainsLine` exists and passes.
- [x] `TestRefDefOnLine` exists and passes.
- [x] `TestLocateInFrontMatter` exists and passes.
- [x] `isGlobPattern` carries "// no test by design".
- [x] `go test ./internal/index/...` — green.
- [x] `go test ./...` — green.
- [x] `mdsmith check .` — 0 failures.
