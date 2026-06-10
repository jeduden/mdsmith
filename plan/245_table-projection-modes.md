---
id: 245
title: "Table projection modes: `records` and `rows`"
status: "🔲"
summary: >-
  Name the current header-keyed object projection
  `projection: records`, add `projection: rows` emitting a
  `columns` array plus row arrays that preserve column order
  and tolerate duplicate headers, and define the
  duplicate-header semantics for both.
model: sonnet
depends-on: []
---
# Table projection modes: `records` and `rows`

## Goal

`kind: table` projects one shape: an array of objects keyed
by column header. That shape loses column order (output keys
are sorted), cannot represent duplicate column headers, and
is awkward for consumers that want positional data — a chart
script, a CSV writer, a diff over runs.

After this plan a table content entry picks one of two
projections. `projection: records` is the current shape and
stays the default:

```json
"matrix": {
  "rows": [
    { "Feature": "check", "Status": "ready" }
  ]
}
```

`projection: rows` emits column order and positional rows:

```json
"matrix": {
  "columns": ["Feature", "Status"],
  "rows": [
    ["check", "ready"]
  ]
}
```

## Design notes

- **Duplicate headers.** `records` reports a duplicate column
  header as an extract-time error (two cells would collide on
  one key). `rows` accepts duplicates — the `columns` array
  is positional.
- **Column order.** `columns` preserves document order; this
  is the only projection surface where order survives the
  sorted-key serializer.
- **Cell text.** Both modes keep today's plain-text cells
  (inline markup flattened, link URLs dropped). Inline-span
  cells ride the block grammar
  ([plan 246](246_block-projection-full-extract.md)).
- Today any `projection:` on a table is rejected at config
  load; this plan narrows that rejection to unknown values.

## Tasks

1. Accept `projection: records | rows` on `kind: table` at
   schema-load time; keep rejecting other values and keep
   `records` the default.
2. Implement the `rows` walker: `columns` from the header
   row in document order, each body row as a same-length
   array (short rows pad with `""`, matching GFM rendering).
3. Define duplicate-header behavior red/green: `records`
   errors with both column positions named; `rows` projects
   them positionally.
4. Document both modes in the
   [extract reference](../docs/reference/cli/extract.md)
   (projection table, duplicate-header semantics) and show
   one `rows` example in the
   [extract guide](../docs/guides/extract-markdown-as-data.md).

## Acceptance Criteria

- [ ] `projection: rows` emits `columns` in document order
  plus positional row arrays; `projection: records` output is
  byte-identical to today's default.
- [ ] A duplicate column header errors under `records` and
  projects under `rows`.
- [ ] A short body row pads with empty strings to the header
  width under `rows`.
- [ ] An unknown table projection still fails at config load.
- [ ] Reference and guide updated with verified outputs.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
