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
|------------------|---------|------------------------------------|
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
- `paragraph` → `text`.

Two sibling projections that resolve to the same key
are a schema error. It is reported at extract time.
Optional sections that did not match are omitted, not
emitted as null.

Renaming or restructuring beyond this default is a
downstream job. Use a tool like `jq` or `yq` over the
standard output. Custom binding overrides are a
separate follow-up.

## Examples

```bash
mdsmith extract recipe --format json recipes/cake.md
mdsmith extract rfc --format yaml docs/rfcs/RFC-0007.md
mdsmith extract plan --format msgpack plan/166_x.md > plan.mp
```

## Exit codes

| Code | Meaning                                                             |
|------|---------------------------------------------------------------------|
| 0    | Extraction succeeded                                                |
| 1    | The file is non-conformant, or a sibling key collision was detected |
| 2    | Runtime or configuration error (unknown kind, kind not assigned, …) |

## See also

- [`mdsmith check`](check.md) — the read-only sibling
  whose clean pass `extract` requires before projecting.
- [Schemas guide](../../guides/schemas.md) — declaring
  the kind schema that doubles as the extraction
  contract.
