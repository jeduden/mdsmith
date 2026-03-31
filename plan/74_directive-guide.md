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

1. Create use-case guides split by topic:

  - `docs/guides/index.md`: guides index with
    generated catalog of all guide pages
  - `docs/guides/directives/generating-content.md`:
    catalog and include use cases with examples,
    placeholder syntax, nesting, and placement
    rules
  - `docs/guides/directives/enforcing-structure.md`:
    schema, require, allow-empty-section use cases
    with composition, schema-vs-normal-file, and
    optional fields
  - `docs/guides/directives/hugo-migration.md`:
    standalone Hugo migration guide with
    placeholder syntax and schema differences
  - `rule-directory.md`: generated catalog of all
    rules from rule READMEs (project root)

2. Add a "see the directive guide" link from
   each rule README that uses a directive
   (MDS019, MDS020, MDS021, MDS030).
3. Replace embedded rules table in README.md with
   a link to the rule directory.
4. Run `mdsmith fix` and `mdsmith check .` to
   verify.

## Acceptance Criteria

- [x] `docs/guides/index.md` exists with generated
      catalog of guides
- [x] Guide covers all four directives with
      examples
- [x] Rule directory generated from rule READMEs
      (replaces manual fixability table per review)
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
