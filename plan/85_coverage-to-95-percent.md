---
id: 85
title: "Increase test coverage to 95% by extracting shared rule helpers"
status: "🔳"
summary: "Refactor duplicated private helpers into shared packages, then test the shared code once."
---
# Increase test coverage to 95% by extracting shared rule helpers

## Goal

Reach 95%+ combined test coverage (unit + e2e) by
eliminating duplicated private helper functions across
rule packages. Instead of testing 17 copies of the same
`toInt`/`headingLine` pattern, extract them into shared
packages and test once.

## Background

Current combined coverage is 89.4%. Of the remaining
gap, 40% comes from ~31 duplicated private helper
functions spread across 15+ rule packages. Each copy
sits at 40--75% coverage because fixture tests only
exercise the `int` branch of `toInt` (missing `float64`,
`int64`) and the ATX branch of `headingLine` (missing
setext, fallback).

## Tasks

Phase 1 --- type conversion helpers:

  - [x] Create `internal/rules/settings/settings.go`
    with exported `ToInt(v any) (int, bool)`,
    `ToFloat(v any) (float64, bool)`, and
    `ToStringSlice(v any) ([]string, bool)`
  - [x] Write table-driven tests in
    `internal/rules/settings/settings_test.go` covering
    all type branches (`int`, `float64`, `int64`,
    `string`, `bool`, `nil`)
  - [x] Replace private `toInt` in 10 rule packages
    (tablereadability, tableformat, tokenbudget,
    linelength, paragraphreadability,
    paragraphstructure, concisenessscoring,
    nomultipleblanks, maxfilelength, firstlineheading)
    with `settings.ToInt`
  - [x] Replace private `toFloat` in 4 rule packages
    (tablereadability, tokenbudget,
    paragraphreadability, concisenessscoring) with
    `settings.ToFloat`
  - [x] Handle the `emptysectionbody` variant (has extra
    `int64` validation) --- decide whether to fold that
    branch into the shared version or keep it local
    (decision: kept local because it rejects non-whole
    floats, which the shared `ToInt` accepts and
    truncates; same rationale for `maxsectionlength`)
  - [x] Delete the now-unused private functions and
    their per-package coverage tests

Phase 2 --- AST heading and paragraph helpers:

  - [ ] Create `internal/rules/astutil/astutil.go` with
    `HeadingLine(h, f)`, `ParagraphLine(p, f)`,
    `IsTable(p, f)`, `HeadingText(h, src)`, and
    `ExtractText(n, src, buf)`
  - [ ] Write tests in
    `internal/rules/astutil/astutil_test.go` covering
    setext headings, ATX headings, the fallback-to-1
    path, table detection, and nested-emphasis
    extraction
  - [ ] Replace 4 identical `headingLine` copies
    (blanklinearoundheadings, noduplicateheadings,
    headingincrement, notrailingpunctuation) with
    `astutil.HeadingLine`; keep the extended variants in
    headingstyle, firstlineheading, and emptysectionbody
    local
  - [ ] Replace 3 identical `paragraphLine` copies and 3
    identical `isTable` copies with
    `astutil.ParagraphLine` and `astutil.IsTable`
  - [ ] Replace `headingText` + `extractText` in
    noduplicateheadings and notrailingpunctuation; keep
    the headingstyle extended version local
  - [ ] Delete unused private functions and their
    per-package coverage tests

Phase 3 --- error-path tests for remaining gaps:

  - [ ] `internal/fix`: test max-passes boundary (10
    iterations without convergence)
  - [ ] `internal/lint`: test `resolveGlob` with invalid
    patterns, `NewGitignoreMatcher` with malformed
    gitignore files
  - [ ] `internal/rules/include`: test `readFSFile` with
    nonexistent and unreadable files
  - [ ] `cmd/mdsmith`: test `formatDiagnostics` write
    error via the error-writer pattern
  - [ ] `internal/rules/requiredstructure`: test
    `cueExprForValue` with `[]any` and `map[string]any`
    inputs; test `extractYAML` with unclosed front
    matter

Phase 4 --- deep edge cases (pursue only if 95% not met
after phases 1--3):

  - [x] `requiredstructure`: CUE value types (`[]any`,
    `map[string]any`), `extractYAML` unclosed front
    matter, `writeNodeText` CodeSpan branch,
    `advanceToMatch` no-match path, `extractPIFileParam`
    multi-line PI
  - [x] `crossfilereferenceintegrity`: anchor-only refs,
    encoded URLs, `DefaultSettings`, `configDiag` via
    invalid glob, `toStringSlice` mixed types
  - [x] `concisenessscoring/classifier`: `validateArtifact`
    field validation (empty model_id, version, threshold,
    weights), `compileLexicon` per-list errors
  - [x] `catalog`: `Category`, `scanIncludesForTarget`
    fallback paths (max depth, read error, no includes,
    direct match, cycle skip), `resolveGitignore`
    param variations

Run linter and tests after every phase:

  - [x] `go test ./...` passes
  - [x] `go tool golangci-lint run` reports no issues

## Acceptance Criteria

- [ ] Combined coverage (unit + e2e) reaches 95%
- [ ] No private `toInt`/`toFloat` copies remain in the
  10 + 4 packages listed above (replaced by
  `settings.ToInt`/`settings.ToFloat`)
- [ ] No private `headingLine`/`paragraphLine` copies
  remain for the 4-copy and 3-copy groups (replaced by
  `astutil.HeadingLine`/`astutil.ParagraphLine`)
- [ ] Shared packages have 100% statement coverage
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] Mutation testing on shared helpers kills 95%+
