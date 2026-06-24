---
id: 2606240212
title: Add dedicated unit tests for lsp/rename.go helpers
status: "🔲"
model: sonnet
summary: >-
  internal/lsp/rename.go has 15 unexported
  helpers without dedicated unit tests. Add a
  named test for each so the audit policy is
  satisfied.
---
# Add dedicated unit tests for lsp/rename.go helpers

## Goal

Add a named unit test for each of the 15 unexported
helpers in `internal/lsp/rename.go`. The 2026-06-24
audit requires it.

## Background

Go arch doc §"Tests" requires every production function
to have a dedicated test by name. The 2026-06-24 audit
(range: 1599c9f..09f22d3) flagged this file.

`atxHeadingTextByteRange` already has a test. The
three trivial pass-through methods on the workspace
adapter carry exemption comments. The 15 helpers
below need dedicated tests:

- `isValidRefDefLine`
- `headingPrepareRange`
- `atxHeadingTextStart`
- `trimTrailingHashRun`
- `skipLeadingSpaces`
- `trimRightSpace`
- `trimmedRange`
- `refDefPrepareRange`
- `refDefBracketBytes`
- `refUsePrepareRange`
- `refUseLabelBytes` (one partial test exists;
  add broader coverage)
- `matchLeadingPair`
- `matchTrailingPair`
- `normalizedLabel`
- `bracketPairs`

## Tasks

1. For each function above, add at least one
   `TestFunctionName` in
   `internal/lsp/rename_test.go`. Drive the helper
   directly with a byte-slice input.
2. `go test ./internal/lsp/...` passes.
3. `go vet ./internal/lsp/...` passes.

## Acceptance Criteria

- [ ] `rename_test.go` contains
      `TestisValidRefDefLine`,
      `TestheadingPrepareRange`,
      `TestatxHeadingTextStart`,
      `TesttrimTrailingHashRun`,
      `TestskipLeadingSpaces`,
      `TesttrimRightSpace`, `TesttrimmedRange`,
      `TestrefDefPrepareRange`,
      `TestrefDefBracketBytes`,
      `TestrefUsePrepareRange`,
      `TestrefUseLabelBytes`,
      `TestmatchLeadingPair`,
      `TestmatchTrailingPair`,
      `TestnormalizedLabel`, `TestbracketPairs`.
- [ ] `go test ./internal/lsp/...` is green.
- [ ] `mdsmith check .` is green.
