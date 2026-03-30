---
id: 74
title: Directive guide
status: "✅"
summary: >-
  Central guide for all directives and rules
  with examples, fixability table, placement
  rules, and nesting behavior.
---
# Directive guide

Part of the user-model work from
[plan 73](73_unify-template-directives.md).
Sibling plans:

- [75](75_single-brace-placeholders.md) --
  `{field}` syntax
- [76](76_rename-misleading-params.md) --
  param renames
- [77](77_template-composition-and-cycles.md) --
  composition, cycles, `schema` rename

Addresses
[#68](https://github.com/jeduden/mdsmith/issues/68),
[#70](https://github.com/jeduden/mdsmith/issues/70).

## Goal

One guide a developer reads to understand every
directive and rule without consulting per-rule
READMEs. Must also serve Hugo users per
[#73](https://github.com/jeduden/mdsmith/issues/73).

Depends on: plans 75, 76, 77 (guide documents
the final syntax, parameter names, and schema
composition). Write the guide last so it
reflects all changes:

- Plan 75: `{field}` replaces `{{.field}}` in
  both catalog rows and schema headings. One
  syntax everywhere. Go templates removed from
  user-facing surface.
- Plan 76: `ratio` -> `words-per-token`,
  `max-words` -> `max-words-per-sentence`,
  `max-column-width-variance` -> `max-column-width-ratio`;
  "did you mean?" hint for case-mismatched
  front-matter keys in catalog.
- Plan 77: config key `template` -> `schema`;
  `<?include?>` works in schema files for
  composition; cycle detection with max depth
  10; `<?require?>` in non-schema files emits
  a warning.

## Context

Blind trials (plan 73) showed six gaps that
docs alone can close:

- 4-space indent silently breaks directives
  (confidence 2.6, no diagnostic emitted).
- Nested directives are undefined (confidence
  2.0, nobody could predict behavior).
- Users cannot predict which rules auto-fix
  (fix confidence 2-3 points lower than check).
- `<?require?>` in a normal file is silently
  ignored (5/5 flagged as confusing -- looks
  like it should work anywhere).
- `<?allow-empty-section?>` in a template does
  not propagate to documents using that
  template (5/5 noted the misleading
  co-occurrence with `## ...`).
- Templates only enforce headings and front
  matter, not directives (2/5 uncertain
  whether `<?catalog?>` in a template requires
  documents to also contain one).

## Rendering note

Processing instructions (`<?...?>`) are hidden
by GitHub's Markdown renderer (CommonMark
type-3 HTML blocks). Directives stay invisible
in rendered docs. Generated content between
markers is visible.

## Tasks

1. Create `docs/guides/directives.md` with:

  - Quick-reference table: name, purpose,
    closing tag (yes/no), fixable (yes/no),
    parameters
  - Two kinds of markers: closing tag means
    `fix` regenerates body; no closing tag means
    `check` validates a condition
  - Placement rules: PIs are only recognized at
    document root (parent must be Document node),
    max 3-space indent. Not recognized inside
    list items, blockquotes, tables, fenced code,
    or HTML blocks
  - Explicit warning: 4+ leading spaces at
    document root turns directive into a code
    block with no error; inside other block
    constructs directives are never recognized
  - Each directive section: purpose, all
    parameters, good example, bad example, what
    `check` reports, what `fix` does
  - Nesting: same-type nesting is prohibited
    (emits "nested generated section markers
    are not allowed"). Cross-type directives
    between markers may appear but can be
    overwritten by the outer generator on fix.
    Generated output is not re-parsed for
    further directives
  - Placeholder syntax section: `{field}` is
    the only syntax, works in both catalog rows
    and schema headings (plan 75). CUE paths
    for nested access (plan 79)
  - Schema vs normal file section: explain
    that `<?require?>` only works in schema
    files (plan 77 adds a warning for misuse),
    that `<?allow-empty-section?>` does not
    propagate from schema to document, and
    that schemas enforce headings and front
    matter only (not directives)
  - Schema composition: `<?include?>` in
    schema files splices headings (plan 77);
    cycle detection with max depth 10
  - Fixability summary table for all 33 rules
  - Renamed parameters: `words-per-token`,
    `max-words-per-sentence`,
    `max-column-width-ratio` (plan 76)
  - "Coming from Hugo" section covering:
    `{title}` not `{{ .Title }}` (case-
    sensitive, no Go templates) with "did you
    mean?" hint (plan 76), `schema` is a
    validation contract not a rendering
    template (plan 77), generated content is
    committed to git (not gitignored), schema
    files compose via `<?include?>` (plan 77),
    no nesting in normal files, `{field}` uses
    CUE paths not Go template syntax, directive
    params are YAML strings
    (per [#73](https://github.com/jeduden/mdsmith/issues/73))

2. Add a "see the directive guide" link from
   each rule README that uses a directive
   (MDS019, MDS020, MDS021, MDS030).
3. Run `mdsmith fix docs/guides/directives.md`
   to regenerate any catalog/include sections.
4. Run `mdsmith check docs/guides/` to verify.

## Acceptance Criteria

- [x] `docs/guides/directives.md` exists
- [x] Guide covers all four directives with
      examples
- [x] Guide has fixability table for all 33
      rules
- [x] Guide documents 4-space indent footgun
- [x] Guide states nesting is not supported
- [x] Guide documents unified `{field}` syntax
- [x] Guide has schema-vs-normal-file section
- [x] Guide states `<?require?>` is
      schema-only
- [x] Guide states schema directives do not
      propagate to documents
- [x] Guide documents schema composition via
      `<?include?>`
- [x] Guide documents renamed parameters
- [x] Guide has "coming from Hugo" section
- [x] Guide uses `schema` not `template`
      throughout
- [x] Guide passes `mdsmith check docs/guides/`
- [x] Rule READMEs link to the guide
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues
