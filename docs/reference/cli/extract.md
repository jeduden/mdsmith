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

`<kind>` must be one of the file's resolved kinds. Extraction
is gated on a successful schema match: a non-conformant file
prints the same diagnostics as `mdsmith check` on stderr,
colored by the [`NO_COLOR` and `FORCE_COLOR` rules](check.md#color), and exits
non-zero, never emitting partial data.

## Flags

| Flag             | Default | Description                        |
| ---------------- | ------- | ---------------------------------- |
| `-f`, `--format` | `json`  | Output format: json, yaml, msgpack |

## Default projection

The projection walks the composed schema in lockstep
with the validated match and mirrors the hierarchy:

- The root holds a `frontmatter` object (the decoded
  front matter) and the projected sections beside it.
- When the schema roots at H2 (all inline schemas do),
  the document H1's plain text is emitted under the
  reserved `title` key beside `frontmatter`. No H1 omits
  the key. A sibling scope that resolves to `title` is
  reported as a collision; rename it with `bind:`.
- A literal heading (`## Goal`) becomes an object keyed
  by the slugified heading (`goal`).
- A repeating section (`## Step {n}` with a `repeat:`
  cardinality) becomes an array keyed by the slug of the
  heading's literal stem (`step`), or the placeholder
  name when the heading is only a placeholder. Each
  element keeps its captured placeholders, child scopes,
  and content.
- A `heading: null` no-heading section projects its
  content directly into the enclosing object — there is
  no `preamble` wrapper key.
- Wildcard slots (`regex: '.+'`) and unlisted or closed
  headings are skipped by default: the output is a
  faithful image of the *declared* schema. A
  schema-level [`projection: blocks`](#block-projection-whole-body)
  lifts that — it projects every section.
- H1-rooted schemas (file-based proto.md schemas where
  the top heading is H1) do not emit the reserved
  `title` key; the H1 is already a scope in `Sections`.

Content entries project under default keys:

- `code-block` → `code` (raw body; more blocks get
  `code-2`, …).
- `list` → `items` (an array of own-text strings), or a
  tree of item objects under `projection: tree` (below).
- `table` → `rows` (default `records`: row objects keyed
  by column header). With `projection: rows` the table
  injects `columns` and `rows` (positional) as two
  sibling keys into the section object instead (below).
- `paragraph` → `text` (plain text), or `inline` when
  the entry sets `projection: inline` (see below).

A flat `items` string holds the item's own text only; a
nested sub-list is excluded, so `- a` with child `- b`
projects `"a"`, never `"ab"`. Use `projection: tree`
(below) to keep nesting and split the task marker out.

Sibling keys are emitted in sorted order, not document
order. Two sibling projections that resolve to the same
key are a schema error, reported at extract time. An
unmatched optional section is omitted, not null; a
section with no `content:` entry projects as `{}`.

## Inline-span projection

A paragraph entry projects its plain text by default.
Set `projection: inline` on the entry
(`{ kind: paragraph, projection: inline }`) to project
the paragraph's inline structure instead — a typed,
recursive list of spans under the `inline` key.

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

Leaf spans (text, code, autolink) carry `value`;
container spans (emphasis, strong, link) carry
`children` and recurse. A link omits `title` when none
was written. A wrapped paragraph keeps line structure:
a text span, then a `break` span (`hard: true` for
backslash/double-space, `false` for soft wrap), then
the next text span.

The headline `Mark*down*, smithed.` projects under
`inline` as a text span, then an `emphasis` span whose
`children` hold the `down` text, then a trailing text
span — see the [guide][igd] for the full output.

[igd]: ../../guides/extract-markdown-as-data.md#projecting-inline-structure

Nesting composes through the same shape: a strong span
wrapping a code span (``**`mdsmith fix`**``) carries
the code span in its `children`, no mode switch.

Each kind limits which projection it takes. A bad pair
fails when the config loads, not later at extract:

| Kind         | Allowed `projection`            |
| ------------ | ------------------------------- |
| `paragraph`  | `text`, `inline`                |
| `code-block` | `code`                          |
| `list`       | `tree` (flat string if omitted) |
| `table`      | `records` (default), `rows`     |
| `unlisted`   | none                            |

A node outside the table — an image, inline raw HTML, a
custom node — is a hard error at extract time, the same
exit code as a non-conformant file. (The block-mode
inline option below is lenient about images.) The `text`
and `inline` default keys differ, so one paragraph can
project each without colliding.

## Tree projection for lists

A list entry projects an array of own-text strings by
default. Set `projection: tree` on a `kind: list` entry
to project each item as an object instead. Each object
carries:

- `text` — the item's own inline text, flattened, the
  task marker removed.
- `checked` — a bool, only on a GFM task item
  (`- [x]` / `- [ ]`). Detection matches the renderer; a
  non-marker word like `[TODO]` stays in `text`.
- `children` — a recursive item array, only when the
  item nests a sub-list.

A checked task nesting one plain child projects as:

```json
{ "checked": true, "children": [{ "text": "child" }], "text": "done" }
```

Array order is item order; ordered-list numbering is out
of scope. YAML and msgpack emit the same tree. The
[guide](../../guides/extract-markdown-as-data.md) has a
worked checklist.

## Table projection modes

A `kind: table` content entry picks one of two
`projection` values. The default is `records`.

| Projection | Output shape                                         |
| ---------- | ---------------------------------------------------- |
| `records`  | `rows: [{Col1: val, Col2: val}, …]`                  |
| `rows`     | `columns: [Col1, Col2, …]` + `rows: [[val, val], …]` |

**`records` (default)** — each body row is an object
keyed by column header. Output key is `rows`. A
duplicate column header is an extract-time error (two
cells would collide on the same key).

**`rows`** — the table injects two sibling keys into the
enclosing section object: `columns` (header strings in
document order) and `rows` (string arrays, one per body
row). Short rows are padded with `""` to header width.
Duplicate headers are accepted — `columns` is positional.

A `Feature`/`Status` table, default vs.
`projection: rows`:

```json
"matrix": { "rows": [{ "Feature": "check", "Status": "ready" }] }
"matrix": { "columns": ["Feature", "Status"], "rows": [["check", "ready"]] }
```

## Block projection (whole-body)

`projection: blocks` projects a section's whole body as
a typed, recursive `blocks` list, in document order. It
is the block-level analogue of `projection: inline`. Set
it on a scope, or once at the schema root as the default
for every section:

```yaml
sections:
  - heading: { regex: '^Notes$' }
    projection: blocks
```

Each body node maps to one block object:

| Body node      | Emitted block                              |
| -------------- | ------------------------------------------ |
| paragraph      | `{block: paragraph, text}`                 |
| fenced code    | `{block: code, lang?, value}`              |
| list           | `{block: list, items: [tree items]}`       |
| table          | `{block: table, columns, rows}`            |
| blockquote     | `{block: quote, blocks: [...]}`            |
| thematic break | `{block: break}`                           |
| HTML block     | `{block: html, value}`                     |
| deeper heading | `{block: section, level, heading, blocks}` |

Container blocks (`quote`, `section`) recurse through
the same grammar. A `section` block appears only for a
heading deeper than the declared schema. Declared
child scopes keep projecting as keyed objects. `code`
keeps its trailing newline; `items` reuse the `tree`
shape above.

A **schema-level** `projection: blocks` also projects
the sections the walker skips — wildcard and unlisted
headings. Each lands under its slug, its heading text in
a `heading` field, a repeated heading as an array. With
the [`title`](#default-projection) key, one switch
yields the whole document as data.

```json
"background": {
  "heading": "Background",
  "blocks": [{ "block": "paragraph", "text": "Why it exists." }]
}
```

Paragraph blocks default to flat `text`. Set
`block-paragraphs: inline` beside `projection: blocks`
to project each paragraph's span list under `inline`
instead. Block-mode inline is lenient: an image
projects an `{span: image, url, …}` span rather than
the hard error strict `projection: inline` raises.

### The CUE contract

The grammar ships as a CUE definition at
`github.com/jeduden/mdsmith/extract`. It is a closed
`#Block` disjunction plus the `#Span` from
[inline projection](#inline-span-projection). A
differential test validates every fixture against it.
The shape cannot drift from this reference.

## Custom binding with `bind`

A scope or content entry can set an optional
`bind:` field to override the default key:

- `bind: <name>` renames a scope's key (replacing
  the slugified heading) or a content entry's key
  (replacing `code` / `inline` / `items` / `rows` / `text`).
- `bind: ""` on a scope hoists its children and content
  into the parent — for a wrapper heading that should not
  nest in the data tree.

Misuses each surface as an error before extraction
runs: duplicate sibling binds, `bind:` on a preamble,
slot, or broad matcher, `bind: ""` on a content entry,
or a real disagreement between composed kinds. For
transformations beyond `bind:`, pipe the output through
`jq` or `yq`.

## Examples

```bash
mdsmith extract recipe --format json recipes/cake.md
mdsmith extract rfc --format yaml docs/rfcs/RFC-0007.md
mdsmith extract plan --format msgpack plan/166_x.md > plan.mp
```

## Exit codes

| Code | Meaning                                                                                       |
| ---- | --------------------------------------------------------------------------------------------- |
| 0    | Extraction succeeded                                                                          |
| 1    | The file is non-conformant, or a sibling key collision was detected                           |
| 2    | Runtime or configuration error (unknown kind, kind not assigned, …), or a failed stderr write |

## See also

- [`mdsmith check`](check.md) — the read-only sibling
  whose clean pass `extract` requires before projecting.
- [Schemas guide](../../guides/schemas.md) — declaring
  the kind schema that doubles as the extraction
  contract.
- [Extract Markdown as data](../../guides/extract-markdown-as-data.md)
  — when to put a value in frontmatter vs. a body
  section, with a worked example.
