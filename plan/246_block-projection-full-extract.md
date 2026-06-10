---
id: 246
title: "Typed block projection and full-document extract"
status: "🔲"
summary: >-
  A block-level analogue of the inline-span grammar: a section
  projects its whole body as a typed, recursive `blocks` list,
  a schema-level default applies it everywhere (wildcard
  sections included), and a CUE definition of the output
  grammar ships as the documented contract.
model: opus
depends-on: [244, 245]
---
# Typed block projection and full-document extract

## Goal

Extract output is a faithful image of the *declared* schema
only. A section without `content:` entries projects as `{}`,
wildcard and unlisted sections are skipped, and heading text
is reduced to a lossy slug key. There is no way to say "give
me all heading body content" — the premise of the
[extract guide](../docs/guides/extract-markdown-as-data.md)
stops at declared entries.

Exposing the raw goldmark AST would be unwieldy and would
freeze a third-party API into the output. The precedent is
[plan 212](212_extract-inline-spans.md): a small, documented,
typed grammar — spans for inline content. This plan is the
block-level analogue, plus a default mode that applies it to
every matched section.

A scope-level projection:

```yaml
sections:
  - heading: { regex: '^Notes$' }
    projection: blocks
```

projects the section's whole body, in document order:

```json
"notes": {
  "blocks": [
    { "block": "paragraph", "text": "First paragraph." },
    { "block": "code", "lang": "go", "value": "func F() {}\n" },
    { "block": "quote", "blocks": [
      { "block": "paragraph", "text": "A quoted line." }
    ]},
    { "block": "list", "items": [ { "text": "one item" } ] },
    { "block": "table", "columns": ["A"], "rows": [["1"]] }
  ]
}
```

A schema-level `projection: blocks` makes that the default
for every scope. It also projects the sections the walker
currently skips. A wildcard or unlisted section projects
under its slug, with its heading text preserved:

```json
"background": {
  "heading": "Background",
  "blocks": [ … ]
}
```

With [plan 243](243_extract-h1-title.md)'s `title` key, a
single schema-level switch yields the whole document as data.

## The block grammar

| Body node      | Emitted block                                   |
| -------------- | ----------------------------------------------- |
| paragraph      | `{block: paragraph, text}`                      |
| fenced code    | `{block: code, lang?, value}`                   |
| list           | `{block: list, items: [tree items]}` (plan 244) |
| table          | `{block: table, columns, rows}` (plan 245)      |
| blockquote     | `{block: quote, blocks: [...]}` (recursive)     |
| thematic break | `{block: break}`                                |
| HTML block     | `{block: html, value}`                          |
| deeper heading | `{block: section, level, heading, blocks}`      |

Container blocks (`quote`, `section`) recurse through the
same grammar. This mirrors the span list's split between
containers and leaves. A `section` block appears only for
headings deeper than the declared schema. Declared child
scopes keep projecting as keyed objects.

## The CUE contract

The output grammar ships as a CUE definition (one `#Block`
disjunction plus the plan-212 `#Span`), published in the
[extract reference](../docs/reference/cli/extract.md). A
differential test validates every extract fixture's JSON
against the CUE definition, so the grammar cannot drift from
the implementation. Consumers get a machine-checkable
contract instead of prose.

## Tasks

1. Implement the block walker in
   [`internal/extract`](../internal/extract) over the grammar
   table above, reusing plan 244's item shape and plan 245's
   `columns`/`rows` shape; paragraphs default to `text`.
2. Accept `projection: blocks` on a scope; the `blocks` key
   joins the default-key set (`bind:`-renamable, collision-
   checked against declared content entries).
3. Accept schema-level `projection: blocks`; project wildcard
   and unlisted scopes under their slug with a `heading` text
   field; repeating matches project as arrays.
4. Write the `#Block` / `#Span` CUE definitions; add the
   differential test validating all extract fixtures against
   them.
5. Offer a paragraph option for inline spans inside blocks
   (`{block: paragraph, inline: [...]}`) gated by the same
   entry-level choice as plan 212, so block mode does not
   force plain text.
6. Document the grammar in the extract reference and rewrite
   the guide's framing: declared entries constrain and
   rename; `projection: blocks` captures everything else.

## Acceptance Criteria

- [ ] `projection: blocks` on a scope projects its full body
  as the typed list, document order preserved, containers
  recursive.
- [ ] Schema-level `projection: blocks` projects wildcard and
  unlisted sections with `heading` text; no section of a
  matched document is silently dropped.
- [ ] Every block fixture validates against the published CUE
  `#Block` definition in a differential test.
- [ ] An HTML block and a thematic break project (no hard
  error); an image inside a `blocks`-mode paragraph projects
  via the inline option or a defined fallback — extract does
  not exit non-zero for representable content.
- [ ] Reference and guide updated with verified outputs.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues

## Out of scope

- Raw-Markdown (verbatim source) projection of a section
  body; revisit if a consumer needs round-tripping rather
  than data.
- Changing the default projection of existing schemas;
  without `projection: blocks` nothing changes.
