---
id: 76
title: Rename misleading parameter names
status: "🔲"
summary: >-
  Rename ratio to words-per-token, max-words
  to max-words-per-sentence, max-column-width-variance
  to max-column-width-ratio, and warn on
  directory-structure no-op.
---
# Rename misleading parameter names

Part of the user-model work from
[plan 73](73_unify-template-directives.md).
Addresses
[#68](https://github.com/jeduden/mdsmith/issues/68)
(user model clarity) and
[#73](https://github.com/jeduden/mdsmith/issues/73)
(Hugo user "did you mean?" hint).

Depends on: none (independent of other plans).
Update plan 74 guide after landing.

## Goal

Each config parameter name tells you what it
measures and what unit it uses.

## Context

Blind trials (plan 73) found three names that
mislead:

- `ratio: 0.75` in token-budget: 2/5 read it
  as a warning threshold. It is a words-to-
  tokens multiplier.
- `max-words: 40` in paragraph-structure: reads
  as per-paragraph limit. It is per-sentence.
- `max-column-width-variance: 60` in
  table-readability: reads as statistical
  variance. It is max/min ratio.

Also found: `directory-structure: true` without
`allowed` is a silent no-op.

## Design

No deprecation. Rename in place, update all
config files and docs in a single PR.

## Tasks

1. Rename `ratio` to `words-per-token` in
   MDS028 (token-budget):

  - Update `ApplySettings` and
    `DefaultSettings` in
    `internal/rules/tokenbudget/rule.go`
  - Update `internal/rules/MDS028-token-budget/README.md`
  - Update `.mdsmith.yml`

2. Rename `max-words` to
   `max-words-per-sentence` in MDS024
   (paragraph-structure):

  - Update
    `internal/rules/paragraphstructure/rule.go`
  - Update `internal/rules/MDS024-paragraph-structure/README.md`
  - Update `.mdsmith.yml`

3. Rename `max-column-width-variance` to
   `max-column-width-ratio` in MDS026
   (table-readability):

  - Update
    `internal/rules/tablereadability/rule.go`
  - Update `internal/rules/MDS026-table-readability/README.md`
  - Update `.mdsmith.yml`

4. Add config warning for MDS033
   (directory-structure) when enabled without
   `allowed`:

  - In `ApplySettings`, if `allowed` is empty
    or absent, emit a diagnostic: "rule enabled
    but no `allowed` patterns configured"
  - Update `internal/rules/MDS033-directory-structure/README.md`

5. Add "did you mean?" diagnostic for
   case-mismatched front-matter keys in catalog
   (MDS019):

  - In catalog `generate`, when a `{{.Field}}`
    lookup returns empty, check for a
    case-insensitive match in the file's front
    matter
  - If found, emit: `catalog: field "Title"
    not found; did you mean "title"?`
  - Hugo users write `{{ .Title }}`; this
    catches the muscle-memory error

6. Update all overrides in `.mdsmith.yml` that
   reference renamed keys.
7. Update `docs/guides/directives.md` and
   `docs/guides/metrics-tradeoffs.md` if they
   reference old names.
8. Run `mdsmith check .` to verify.

## Acceptance Criteria

- [ ] `ratio` renamed to `words-per-token`
- [ ] `max-words` renamed to
      `max-words-per-sentence`
- [ ] `max-column-width-variance` renamed to
      `max-column-width-ratio`
- [ ] `directory-structure: true` without
      `allowed` emits a config warning
- [ ] Case-mismatched front-matter key in
      catalog emits "did you mean?" hint
- [ ] `.mdsmith.yml` uses new names throughout
- [ ] All rule READMEs use new names
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
- [ ] `mdsmith check .` passes
