---
id: 2606240213
title: >-
  Add dedicated unit tests for export.go helpers
  and two small rename helpers
status: "🔲"
model: sonnet
summary: >-
  internal/export/export.go has 11 unexported
  helpers without dedicated tests. Two small
  helpers from concisenessscoring and rename are
  batched here. Flagged by the 2026-06-24 audit.
---
# Add dedicated unit tests for export helpers

## Goal

Add named unit tests for 11 unexported helpers in
`internal/export/export.go`. Two small helpers from
other files are batched here as well.

## Background

Go arch doc §"Tests" requires every production function
to have a dedicated test by name. The 2026-06-24 audit
(range: 1599c9f..09f22d3) flagged all three files.

### internal/export/export.go

Functions without a dedicated test:

- `selectDirectives` — pick rules with directives
- `allDirectiveNames` — list built-in directive names
- `regenerate` — rebuild generated sections
- `hydrate` — copy diagnostics to original file
- `checkStaleness` — flag stale generated bodies
- `inGeneratedRange` — check if line is generated
- `stripDirectives` — remove directive PI markers
- `piLineRange` — compute PI block line range
- `overlapsAny` — check if `[from,to]` hits line set
- `emitLines` — emit non-stripped lines to buffer
- `normalizeBlankLines` — collapse blank-line runs

### internal/rules/concisenessscoring/rule.go

- `countClassifierTokens` — count tokens without
  allocating a regex match per call

### internal/rename/rename.go

- `contentBlockLines` — lines inside code blocks

## Tasks

1. Add a `TestFunctionName` in
   `internal/export/export_test.go` for each of the
   11 export helpers above.
2. Add `TestcountClassifierTokens` in
   `internal/rules/concisenessscoring/rule_test.go`.
3. Add `TestcontentBlockLines` in
   `internal/rename/rename_test.go`.
4. `go test ./internal/export/...` passes.
5. `go test ./internal/rules/concisenessscoring/...`
   passes.
6. `go test ./internal/rename/...` passes.

## Acceptance Criteria

- [ ] `export_test.go` contains
      `TestselectDirectives`, `TestallDirectiveNames`,
      `Testregenerate`, `Testhydrate`,
      `TestcheckStaleness`, `TestinGeneratedRange`,
      `TeststripDirectives`, `TestpiLineRange`,
      `TestoverlapsAny`, `TestemitLines`,
      `TestnormalizeBlankLines`.
- [ ] `rule_test.go` contains
      `TestcountClassifierTokens`.
- [ ] `rename_test.go` contains
      `TestcontentBlockLines`.
- [ ] All three `go test` commands are green.
- [ ] `mdsmith check .` is green.
