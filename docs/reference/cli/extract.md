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
- `list` → `items` (an array of strings, each the item's
  own text), or a tree of item objects when the entry
  sets `projection: tree` (see below).
- `table` → `rows` (default `records` projection: row
  objects keyed by column header). With `projection: rows`
  the table injects `columns` (header array) and `rows`
  (positional row arrays) as two sibling keys into the
  enclosing section object instead (see below).
- `paragraph` → `text` (plain text), or `inline` when
  the entry sets `projection: inline` (see below).

A flat `items` string holds the item's own text only.
A nested sub-list is excluded, not folded in, so `- a`
with child `- b` projects `"a"`, never `"ab"`. Inline
markup flattens to text; a task marker stays verbatim
(`[x]` / `[ ]`); a nest-only item projects the empty
string, keeping its slot. Use `projection: tree` (below)
to keep the nesting and split the marker out.

Sibling keys are emitted in sorted order, not document
order. Two sibling projections that resolve to the same
key are a schema error. It is reported at extract time.
Optional sections that did not match are omitted, not
emitted as null. A section that declares no `content:`
entry projects as an empty object.

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

Leaf spans (text, code, autolink) carry `value`;
container spans (emphasis, strong, link) carry
`children` and recurse through the same mapping.
A link omits `title` when none was written.

A wrapped paragraph keeps line structure: a text span,
then a `break` span (`hard: true` for backslash/double-
space, `false` for soft wrap), then the next text span.

For the headline `Mark*down*, smithed.`:

```json
"headline": {
  "inline": [
    { "span": "text", "value": "Mark" },
    {
      "children": [{ "span": "text", "value": "down" }],
      "level": 1,
      "span": "emphasis"
    },
    { "span": "text", "value": ", smithed." }
  ]
}
```

A nested example — a strong span wrapping a code span,
``**`mdsmith fix`**`` — projects with no mode switch:

```json
{
  "children": [{ "span": "code", "value": "mdsmith fix" }],
  "level": 2,
  "span": "strong"
}
```

Each kind limits which projection it takes. A bad pair
fails when the config loads, not later at extract:

| Kind         | Allowed `projection`            |
| ------------ | ------------------------------- |
| `paragraph`  | `text`, `inline`                |
| `code-block` | `code`                          |
| `list`       | `tree` (flat string if omitted) |
| `table`      | `records` (default), `rows`     |
| `unlisted`   | none                            |

Anything outside the mapping table is a hard error at
extract time, with the same exit code as a non-conformant
file: images, inline raw HTML, and custom inline nodes.
The `text` and `inline` default keys differ, so one
paragraph can project each without colliding.

## Tree projection for lists

A list entry projects an array of own-text strings by
default. Set `projection: tree` on a `kind: list` entry
to project each item as an object instead. Each object
carries:

- `text` — the item's own inline text, flattened, with
  soft wraps joined and any task marker removed.
- `checked` — a bool, present only on a GFM task item
  (`- [x]` / `- [ ]`). Detection matches the renderer: a
  bare `[x]` and a no-space `[x]done` count; a non-marker
  word like `[TODO]` does not and stays in `text`.
- `children` — a recursive array of item objects, present
  only when the item nests a sub-list.

A checked task nesting one plain child projects as:

```json
{ "checked": true, "children": [{ "text": "child" }], "text": "done" }
```

The array order is the item order; ordered-list numbering
and `start` are out of scope. YAML and msgpack emit the
same tree. The
[extract guide](../../guides/extract-markdown-as-data.md)
has a full worked checklist.

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

For a `Feature`/`Status` table with `{ kind: table }`:

```json
"matrix": { "rows": [{ "Feature": "check", "Status": "ready" }] }
```

With `{ kind: table, projection: rows }`:

```json
"matrix": {
  "columns": ["Feature", "Status"],
  "rows": [["check", "ready"]]
}
```

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
