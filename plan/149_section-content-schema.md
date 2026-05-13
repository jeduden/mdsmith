---
id: 149
title: Section content schema for non-heading AST nodes
status: "✅"
model: opus
depends-on: [146]
summary: >-
  Add a `content:` field at the section scope
  level so a schema can declare a positional
  sequence of required AST node entries
  (code blocks of a given language, tables with
  named columns, lists with item bounds,
  paragraphs) inside a matched section. The
  `sections:` array stays heading-rooted; the
  new `content:` array carries the heterogeneous
  AST-node constraints one level deeper.
---
# Section content schema for non-heading AST nodes

## Goal

Let a schema describe what must appear inside
a matched section. A scope can list required
code blocks, tables, lists, and paragraphs.
The validator walks the section's AST body in
order. Missing, extra, or out-of-order nodes
surface alongside the existing heading-tree
diagnostics.

## Background

Plan 146 ships the scope tree with one
positional list: `sections:`. Every entry is
heading-rooted. Plan 142 covers prose
constraints as per-scope rules — forbidden
text, required mentions, word caps. That plan
deliberately introduces no new schema shape;
it routes through the existing `rules:`
override block.

The remaining gap is positional AST-node
constraints below the heading level. Examples:
"Examples must contain a YAML code block";
"Settings must contain a table with `Setting`
and `Default` columns"; "Diagnosis steps must
end with a paragraph". These constraints are
position-sensitive within the section, so they
can't be expressed as document-wide rules. And
they aren't headings, so `sections:` doesn't
fit. They live as a second positional array on
the scope, separate from `sections:`.

The shape choice is settled. The `sections:`
array stays uniformly heading-rooted; the new
`content:` field carries non-heading AST nodes
one level deeper inside a section. Mixing
both into one heterogeneous array (`kind:`
discriminator on every entry) was rejected as
overly verbose for the common case and as
conflating layout-of-headings with
content-within-a-section. See plan 146's
discussion of section-array shape.

## Non-Goals

- Content-of-content: nested constraints
  inside a code block, table cells, list
  items. Plan 142 covers prose-text rules;
  this plan covers node-shape constraints
  only.
- Schema for CUE-typed table rows. Tables get
  required column NAMES today; values are out
  of scope.
- Frontmatter-body sync against `content:`
  entries (the `{field}` interpolation in
  file-based schemas stays heading-only).
- Auto-fix for missing content nodes.

## Design

### A second positional array

Each scope in `sections:` may carry a
`content:` list alongside its existing
`heading:` / `sections:` / `closed:` / `rules:`
keys. The validator runs `content:`
validation only when the scope matched a doc
heading; the search is bounded by the
section's line range (same window the
per-scope rule walker uses).

```yaml
sections:
  - heading: "Examples"
    required: true
    content:
      - kind: code-block
        lang: yaml
      - kind: paragraph
        required: false
```

### Entry kinds

| `kind:`      | Extra keys (all optional)                  | Matches                                       |
|--------------|--------------------------------------------|-----------------------------------------------|
| `code-block` | `lang:` (string; exact match today)        | An `ast.FencedCodeBlock` whose info matches.  |
| `table`      | `columns: [str, ...]` (exact header names) | A GFM table whose first row equals `columns`. |
| `list`       | `ordered: bool`, `min-items`, `max-items`  | An `ast.List` whose ordered flag matches.     |
| `paragraph`  | (none today)                               | An `ast.Paragraph` not recognised as a table. |
| `unlisted`   | (none)                                     | A positional slot — see below.                |

Every entry accepts `required: true|false`.
The default is `true`. The parser rejects
`required:` on `kind: unlisted` outright.

The parent scope's `closed:` flag drives
diagnostics for trailing or intervening
unlisted nodes. A per-entry `closed:` field
is not shipped. Entries name single AST
blocks; there is no child content to be
closed against.

### Wildcards and order

`content:` follows the same out-of-order +
unlisted-slot semantics the scope-tree
validator already implements:

- Entries match in declared order; later
  entries can be claimed out-of-order when
  the doc has them early, with a diagnostic.
- `unlisted: true` is the positional slot
  (same as plan 146's section-array slot,
  renamed from `"..."`); intervening
  non-matching nodes inside the slot are
  tolerated; the next listed entry's match
  ends the slot.
- `closed: true` on the parent scope makes
  unlisted nodes outside a `unlisted` slot a
  diagnostic; `closed: false` (default)
  tolerates them silently.

### Diagnostics

All new diagnostics emit through
`lint.Diagnostic`. They reuse the existing
`MDS020 required-structure` rule ID. Messages
name the expected kind and any constraint:

- `missing required content "code-block lang=yaml" inside ## Examples`
- `unexpected content "table" inside ## Examples (expected "paragraph")`
- `content "table" out of order: expected after "code-block lang=yaml"`
- `code block language %q does not match required %q`
- `table headers %v do not match required %v`

The diagnostic line points at the offending
AST node when present; otherwise at the
section's heading line.

## Tasks

1. Define `ContentEntry` plus a
   `Content []ContentEntry` field on `Scope`.
   Each entry carries `Kind`, `Required`, and
   a small set of kind-specific fields
   directly on the struct (`Lang`, `Columns`,
   `Ordered`/`OrderedSet`, `MinItems`,
   `MaxItems`) — the design uses a flat
   discriminated record rather than five
   nested kind-specific sub-structs because
   the constraint sets are tiny and a flat
   record keeps the parser, validator, and
   describer one function per field.
2. Inline parser: extend
   `parseInlineScopeEntry` to read a
   `content:` mapping value; reject unknown
   `kind:` strings; reject content-on-a-
   wildcard / content-on-a-`?`-heading until
   the validator decides what those mean.
3. Validator: after a scope claims its doc
   heading, walk the section's AST body
   (between the heading and the next sibling
   or parent) and match each `ContentEntry`
   in order, mirroring the heading-tree
   matching rules.
4. Diagnostic messages: add the five new
   message templates listed above to MDS020.
   Reuse `formatHeading` / scope-line helpers
   so output stays consistent.
5. File-based parser: leave `proto.md`
   alone for now. The legacy heading-template
   path stays unchanged; `content:` is
   inline-only until plan 146's file-based
   migration follow-up lands.
6. Fixtures: a runbook-style fixture
   exercising `code-block`, `table`, `list`,
   `paragraph`, and `unlisted` inside one
   section; bad fixtures for each mismatch
   diagnostic.
7. Document `content:` in the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md)
   and
   [docs/guides/schemas.md](../docs/guides/schemas.md).

## Acceptance Criteria

- [x] A schema with `content: [{kind:
      code-block, lang: yaml}]` flags a
      matched section missing that block.
- [x] A schema with `content: [{kind:
      table, columns: [Setting, Default]}]`
      flags a section whose table has
      different column headers and passes
      one with matching headers.
- [x] A schema with `content: [{kind: list,
      ordered: true, min-items: 2}]` flags a
      section with a one-item ordered list.
- [x] `closed: true` on the parent scope
      flags an unlisted AST node inside the
      section.
- [x] `kind: unlisted` tolerates unknown
      nodes at that position even under
      `closed: true`.
- [x] Content out-of-order produces the new
      "out of order" diagnostic naming
      expected vs actual kind.
- [x] An unknown `kind:` value at parse
      time emits an error naming the scope.
- [x] `content:` is rejected on a wildcard
      slot and on a `heading: "?"` scope
      until a follow-up plan defines those
      semantics.
- [x] All existing MDS020 fixtures pass
      unchanged.
- [x] MDS020 README and
      `docs/guides/schemas.md` document the
      `content:` shape with one worked
      example.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports
      no issues.
