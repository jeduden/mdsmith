---
id: 75
title: Single-brace placeholders everywhere
status: "✅"
summary: >-
  Replace {{.field}} with {field} in both
  required-structure and catalog. One placeholder
  syntax across the whole tool.
---
# Single-brace placeholders everywhere

Part of the user-model work from
[plan 73](73_unify-template-directives.md).
Addresses
[#68](https://github.com/jeduden/mdsmith/issues/68).

Depends on: none (independent of other plans).
Update plan 74 guide after landing.

## Goal

One placeholder syntax: `{field}`. It works
the same way everywhere -- resolves a
front-matter key by name using CUE path syntax
([plan 79](79_nested-frontmatter-access.md)
adds nested access).

- In catalog `row`/`header`/`footer`: inserts
  the value from matched files' front matter.
- In schema headings: heading must contain the
  value from the document's own front matter.

`{{.field}}` (Go template syntax) is removed
from the user-facing surface entirely.

## Context

Blind trials (plan 73) showed `{{.field}}` was
the top confusion source. The original plan
separated the two syntaxes (`{field}` for
schemas, `{{.field}}` for catalog). But keeping
Go templates in catalog means two grammars.
Aligning to one syntax is simpler.

Go `text/template` is overkill for catalog row
rendering: no user uses conditionals, loops,
or functions in row templates. Simple string
interpolation with `{field}` suffices.

## Rendering note

`{field}` renders as visible literal text on
GitHub. `# {id}: {name}` reads as a clear
pattern. Catalog row params in YAML also read
naturally: `row: "- [{title}]({filename})"`.
No backslash escaping or dot-prefix needed.

## Design

Replace Go `text/template` in catalog with
simple `{field}` interpolation. Same regex,
same resolver, same CUE path semantics as
required-structure.

- `row: "- [{title}]({filename})"` -- catalog
  row rendering.
- `# {id}: {name}` -- schema heading match.
- `{field}` resolves to empty string if the
  key is missing (current behavior preserved).
- `{filename}` remains a built-in field in
  catalog context (relative path).
- Other built-ins: `{title}`, `{summary}`,
  etc. are front-matter lookups, not special.

### What changes from Go templates

| Feature       | Go template         | `{field}`              |
|---------------|---------------------|------------------------|
| Syntax        | `{{.field}}`        | `{field}`              |
| Nested access | `{{.a.b}}`          | `{a.b}` (plan 79)      |
| Quoted keys   | `{{ index . "k" }}` | `{"my-key"}` (plan 79) |
| Conditionals  | `{{ if .x }}`       | Not supported          |
| Loops         | `{{ range }}`       | Not supported          |
| Functions     | `{{ .x \| fn }}`    | Not supported          |
| Missing key   | empty string        | empty string           |

No catalog in the repo uses conditionals,
loops, or functions today.

### Escaping literal braces

Literal `{` is written as `{{`, literal `}` as
`}}`. Same convention as Python's `str.format`.
Example: `row: "{{literal}} {title}"` renders
as `{literal} My Title`.

## Tasks

1. Add a `{field}` interpolation engine in a
   shared package (e.g. `internal/fieldinterp`):

  - Parse `{...}` placeholders from a string
  - Resolve each placeholder against a
    `map[string]any` using CUE path rules
  - Return the interpolated string
  - Handle `{{` as escaped literal `{`

2. Update `catalog/generate.go`:

  - Replace `text/template` rendering with
    `fieldinterp.Interpolate`
  - Keep `{filename}` as a built-in injected
    into the data map before interpolation

3. Update `requiredstructure/rule.go`:

  - Replace `fieldPattern` regex with call to
    the shared `fieldinterp` parser
  - `resolveFields` uses the shared resolver
  - Pattern matching builds regex from parsed
    placeholders (same logic, shared parse)

4. Update unit tests in both rules.
5. Update fixtures:

  - `internal/rules/MDS020-required-structure/`
  - `internal/rules/MDS019-catalog/`
  - Any fixture templates or catalog directives
    using `{{.field}}`

6. Migrate all schema files:

  - `plan/proto.md`
  - `internal/rules/proto.md`
  - `.claude/skills/proto.md`

7. Migrate all catalog directives in the repo
   (CLAUDE.md, README.md, rule READMEs) from
   `{{.field}}` to `{field}`.
8. Update rule READMEs (MDS019, MDS020).
9. Update `docs/guides/directives.md` (plan 74)
   if it already exists.
10. Run `mdsmith check .` to verify.

## Acceptance Criteria

- [x] `{field}` is the only placeholder syntax
      in both catalog and required-structure
- [x] `{{.field}}` is no longer recognized
- [x] All schema files use `{field}`
- [x] All catalog directives use `{field}`
- [x] Shared interpolation engine exists
- [x] Literal `{` is escaped as `{{`
- [x] MDS019 and MDS020 READMEs updated
- [x] All fixtures updated and passing
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues
- [x] `mdsmith check .` passes
