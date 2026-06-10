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
- `table` with columns → `rows` (row objects keyed by
  column header).
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
| `table`      | none                            |
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

A two-section kind schema, a kind assignment, and a
conformant file. Each section declares a `content:`
entry. A section without one projects as an empty
object:

```yaml
# .mdsmith.yml
kinds:
  product-copy:
    schema:
      sections:
        - heading: { regex: '^Tagline$' }
          content:
            - { kind: paragraph }
        - heading: { regex: '^VS Code$' }
          bind: vscode-description
          content:
            - { kind: paragraph }
kind-assignment:
  - glob: ["docs/copy.md"]
    kinds: [product-copy]
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
  "frontmatter": {
    "title": "Product copy"
  },
  "tagline": {
    "text": "Mark down your ideas; smith them into shipping docs."
  },
  "vscode-description": {
    "text": "Inline diagnostics, fix-on-save, and instant navigation for Markdown in VS Code."
  }
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
