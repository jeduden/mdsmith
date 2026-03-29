---
id: 75
title: Single-brace placeholders
status: "🔲"
summary: >-
  Switch required-structure heading patterns from
  {{.field}} to {field} and migrate all template
  files in a single PR.
---
# Single-brace placeholders

Part of the user-model work from
[plan 73](73_unify-template-directives.md).
Addresses
[#68](https://github.com/jeduden/mdsmith/issues/68).

Depends on: none (independent of other plans).
Update plan 74 guide after landing.

## Goal

`{{.field}}` always means "insert value" (Go
template in catalog). `{field}` always means
"heading must contain front-matter value"
(required-structure pattern). No overlap.

## Context

Blind trials (plan 73) showed all five
participants flagged `{{.field}}` dual meaning
as the top confusion source. Three called it
"genuinely confusing" despite getting the
right answer.

## Rendering note

`{field}` renders as visible literal text on
GitHub. This is good: when viewing a template
file (`proto.md`) on GitHub, the placeholder
`# {id}: {name}` reads as a clear pattern.
Compare with `# {{.id}}: {{.name}}` which
looks like a code artifact.

PIs around the placeholder stay hidden on
GitHub (CommonMark type-3 HTML blocks). Only
the heading pattern is visible.

## Design

In a required-structure template heading,
`{field}` means: "the document heading here
must contain the value of front-matter key
`field`."

- `# {id}: {name}` matches `# MDS001: line-length`
  when `id: MDS001` and `name: line-length`.
- `# ?` remains the wildcard for "any heading."
- `# ...` remains the section wildcard.

No deprecation cycle. All templates transition
in a single PR. `{{.field}}` stops being
recognized by required-structure.

## Tasks

1. Update `requiredstructure/rule.go` to parse
   `{field}` instead of `{{.field}}`:

  - Change the regex that extracts placeholder
    names from `\{\{\.(\w+)\}\}` to `\{(\w+)\}`
  - Update the pattern-to-regex builder
  - Update all error messages

2. Update unit tests in
   `requiredstructure/rule_test.go`.
3. Update fixtures:

  - `internal/rules/MDS020-required-structure/`
    good and bad examples
  - Any fixture templates using `{{.field}}`

4. Migrate all schema files (called "template
   files" until plan 77 renames the config key):

  - `plan/proto.md`
  - `internal/rules/proto.md`
  - `.claude/skills/proto.md` (if it exists)

5. Update
   `internal/rules/MDS020-required-structure/README.md`
   to document `{field}` syntax.
6. Update `docs/guides/directives.md` (from
   plan 74) if it already exists.
7. Run `mdsmith check .` to verify all markdown
   passes.

## Acceptance Criteria

- [ ] `{field}` is the only placeholder syntax
      in required-structure
- [ ] `{{.field}}` is no longer recognized by
      the required-structure rule
- [ ] All template files use `{field}`
- [ ] MDS020 README documents `{field}`
- [ ] All fixtures updated and passing
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
- [ ] `mdsmith check .` passes
