---
command: extract
summary: Emit a schema-conformant Markdown file as a JSON/YAML/msgpack data tree.
---
# `mdsmith extract`

Project a schema-conformant Markdown file into a data
tree whose nesting mirrors the kind's schema hierarchy,
and write it to stdout. No schema annotations are
required — the schema is the extraction contract.

```text
mdsmith extract <kind> --format <fmt> <file>
```

`<kind>` must be one of the file's resolved kinds.
Extraction is gated on a successful schema match: a
non-conformant file prints the same diagnostics as
`mdsmith check` and exits non-zero, never emitting
partial data.

## Flags

| Flag             | Default | Description                        |
| ---------------- | ------- | ---------------------------------- |
| `-f`, `--format` | `json`  | Output format: json, yaml, msgpack |

## Default projection

The projection walks the composed schema in lockstep
with the validated match and mirrors the hierarchy:

- The root holds a `frontmatter` object (the decoded
  front matter, unchanged) and the projected sections
  beside it at the same level.
- A literal heading (`## Goal`) becomes an object keyed
  by the slugified heading (`goal`).
- A repeating section (`## Step {n}` with a `repeat:`
  cardinality) becomes an array keyed by the slug of the
  heading's literal stem (`step`), or the placeholder
  name when the heading is only a placeholder. Each
  element retains every captured placeholder as a
  `name: value` field plus its own child scopes and
  content.
- A `heading: null` no-heading section projects its
  content directly into the enclosing object — there is
  no `preamble` wrapper key.
- Wildcard slots (`regex: '.+'`) and unlisted or closed
  headings are skipped: the output is a faithful image
  of the *declared* schema only.

Content entries project under default keys:

- `code-block` → `code` (raw body; more blocks get
  `code-2`, …).
- `list` → `items`.
- `table` with columns → `rows` (row objects keyed by
  column header).
- `paragraph` → `text` (plain text), or `inline` when
  the entry sets `projection: inline` (see below).

Two sibling projections that resolve to the same key
are a schema error. It is reported at extract time.
Optional sections that did not match are omitted, not
emitted as null.

## Inline-span projection

A paragraph entry projects its plain text by default.
Set `projection: inline` on the entry to project the
paragraph's inline structure instead. The result is a
typed, recursive list of spans under the `inline` key:

```yaml
sections:
  - heading: { regex: '^Headline$' }
    content:
      - { kind: paragraph, projection: inline, required: true }
```

Each AST node maps to one span object:

| AST node           | Emitted span                                  |
| ------------------ | --------------------------------------------- |
| text               | `{span: text, value}`                         |
| line break         | `{span: break, hard}`                         |
| code span          | `{span: code, value}`                         |
| autolink (`<url>`) | `{span: autolink, value, url}`                |
| emphasis (`*…*`)   | `{span: emphasis, level: 1, children: [...]}` |
| strong (`**…**`)   | `{span: strong, level: 2, children: [...]}`   |
| link (`[t](url)`)  | `{span: link, url, title?, children: [...]}`  |

Leaf spans (text, code, autolink) carry a `value`;
container spans (emphasis, strong, link) carry a
`children` list and recurse through the same mapping,
so nesting composes uniformly. A link omits `title`
when the Markdown link has none.

A wrapped paragraph keeps its line structure: a text
node that ends in a line break emits its text span and
then a `break` span. `hard` is `true` for a hard break
(a backslash or two trailing spaces before the newline)
and `false` for a soft wrap. So `first⏎second` projects
as `[{span: text, value: first}, {span: break, hard:
false}, {span: text, value: second}]`.

For the headline `Mark*down*, smithed.`:

```json
"headline": {
  "inline": [
    { "span": "text", "value": "Mark" },
    { "span": "emphasis", "level": 1, "children": [
      { "span": "text", "value": "down" }
    ]},
    { "span": "text", "value": ", smithed." }
  ]
}
```

A nested example — a strong span wrapping a code span,
``**`mdsmith fix`**`` — projects with no mode switch:

```json
{ "span": "strong", "level": 2, "children": [
    { "span": "code", "value": "mdsmith fix" }
] }
```

Each content kind constrains its projection at schema-
load time. A `paragraph` takes `text` or `inline`. A
`code-block` takes `code`. A `table`, `list`, or
`unlisted` slot takes none. An incompatible combination
fails when the config loads, not silently at extract
time. Rejected cases include `projection: code` on a
paragraph, `projection: inline` on a code-block, and any
`projection` on a table.

Anything outside the mapping table is a hard error at
extract time, with the same exit code as a non-conformant
file. Images, inline raw HTML, and custom inline nodes
fall in this set. The `text` and `inline` default keys
differ, so one paragraph entry can project `text` and
another `inline` without colliding. A `bind:` override
renames either key.

## Custom binding with `bind`

A scope or content entry can set an optional
`bind:` field to override the default key:

- `bind: <name>` renames a scope's key (replacing
  the slugified heading) or a content entry's key
  (replacing `code` / `inline` / `items` / `rows` / `text`).
- `bind: ""` on a scope hoists its children and
  content directly into the parent — useful when a
  wrapper heading exists only for document structure
  and should not nest in the data tree.

Duplicate sibling binds, `bind:` on a preamble,
slot, or broad matcher, `bind: ""` on a content
entry, and a real disagreement between composed
kinds each surface as an error before extraction
runs. For ad-hoc transformations beyond what
`bind:` covers, pipe the standard output through
`jq` or `yq`.

## Examples

```bash
mdsmith extract recipe --format json recipes/cake.md
mdsmith extract rfc --format yaml docs/rfcs/RFC-0007.md
mdsmith extract plan --format msgpack plan/166_x.md > plan.mp
```

### Worked example

A two-section kind schema and a conformant file:

```yaml
# .mdsmith.yml
kinds:
  product-copy:
    schema:
      sections:
        - heading: { regex: '^Tagline$' }
        - heading: { regex: '^VS Code$' }
          bind: vscode-description
```

```markdown
<!-- docs/copy.md -->
---
title: Product copy
---
# Product copy

## Tagline

Mark down your ideas; smith them into shipping docs.

## VS Code

Inline diagnostics, fix-on-save, and instant
navigation for Markdown in VS Code.
```

`mdsmith extract product-copy --format json docs/copy.md`
emits:

```json
{
  "frontmatter": { "title": "Product copy" },
  "tagline": { "text": "Mark down your ideas; smith them into shipping docs." },
  "vscode-description": { "text": "Inline diagnostics, fix-on-save, and instant navigation for Markdown in VS Code." }
}
```

Each H2 projects as an object keyed by the slugified
heading (or the `bind:` override); the paragraph
under each heading projects as `text`. Frontmatter is
the file's metadata; body sections are the file's
payload. See the
[Extract Markdown as data guide](../../guides/extract-markdown-as-data.md)
for when to put a value in frontmatter vs. a body
section.

## Exit codes

| Code | Meaning                                                             |
| ---- | ------------------------------------------------------------------- |
| 0    | Extraction succeeded                                                |
| 1    | The file is non-conformant, or a sibling key collision was detected |
| 2    | Runtime or configuration error (unknown kind, kind not assigned, …) |

## See also

- [`mdsmith check`](check.md) — the read-only sibling
  whose clean pass `extract` requires before projecting.
- [Schemas guide](../../guides/schemas.md) — declaring
  the kind schema that doubles as the extraction
  contract.
- [Extract Markdown as data](../../guides/extract-markdown-as-data.md)
  — when to put a value in frontmatter vs. a body
  section, with a worked example.
