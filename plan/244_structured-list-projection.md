---
id: 244
title: "Structured list projection; fix nested-item text corruption"
status: "🔲"
summary: >-
  Fix flat list projection concatenating nested-item text into
  the parent with no separator, then add `projection: tree` so
  list items project as objects with `text`, a `checked` bool
  for task items, and recursive `children`.
model: opus
depends-on: []
---
# Structured list projection; fix nested-item text corruption

## Goal

`kind: list` projects items as flat strings. The flattening
is corrupt. A nested child's text concatenates into its
parent with no separator. Inline markup is silently
stripped. For

```markdown
- [x] done item
- [ ] open item with **bold**
  - nested child
```

extract emits

```json
"items": [
  "[x] done item",
  "[ ] open item with boldnested child"
]
```

`boldnested child` is data corruption, not just loss. Task
checkboxes survive only as literal `"[x] "` prefixes, nesting
is gone, and no consumer can recover any of it.

This plan has two parts. First, an unconditional bugfix: a
flat item projects its own text only — children are excluded,
word boundaries survive. Second, an opt-in structured mode:

```yaml
content:
  - { kind: list, projection: tree }
```

projects each item as an object:

```json
"items": [
  { "text": "done item", "checked": true },
  {
    "checked": false,
    "children": [
      { "text": "nested child" }
    ],
    "text": "open item with bold"
  }
]
```

- `text` — the item's own inline text, soft wraps joined.
- `checked` — present only on task-list items
  (`- [x]` / `- [ ]`); the marker leaves `text`.
- `children` — present only when the item nests a sub-list;
  recursive through the same shape.

## Design notes

- The flat default keeps emitting strings (compat), minus the
  corruption: own text only, task markers kept as today.
- `projection: tree` is validated at schema-load time like
  `projection: inline` (list-only; rejected elsewhere).
- Ordered-list metadata (`start`, numbering) is out of scope;
  the item order is the array order either way.
- Inline spans inside items
  (`projection: tree` + span lists) ride the block grammar
  ([plan 246](246_block-projection-full-extract.md)), not
  this plan.

## Tasks

1. **Bugfix first (red/green).** Failing test reproducing
   `boldnested child`; fix flat projection to emit each
   top-level item's own text. Decide and test what flat mode
   emits for an item that is *only* a nested list.
2. Add `projection: tree` to content-entry validation
   (list-only) and implement the recursive item walker in
   [`internal/extract`](../internal/extract): `text`,
   optional `checked`, optional `children`.
3. Task-list detection: `checked` appears only on items with
   a GFM task marker; the marker text never leaks into
   `text` in tree mode.
4. Document both modes and the bugfix in the
   [extract reference](../docs/reference/cli/extract.md)
   (default-projection table + a tree example) and add a
   worked example to the
   [extract guide](../docs/guides/extract-markdown-as-data.md).

## Acceptance Criteria

- [ ] Flat `items` never concatenates nested-item text into a
  parent string; the corrupt case above projects
  `"[ ] open item with bold"`.
- [ ] `projection: tree` emits item objects with `text`,
  `checked` only on task items, `children` only when
  non-empty, recursive to any depth.
- [ ] `projection: tree` on a non-list content entry fails at
  config load.
- [ ] YAML and msgpack formats emit the same tree.
- [ ] Reference and guide updated with verified outputs.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
