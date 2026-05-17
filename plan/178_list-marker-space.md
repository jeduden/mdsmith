---
id: 178
title: List marker space rule
status: "🔲"
model: sonnet
depends-on: []
summary: >-
  New rule MDS061 (provisional) for markdownlint MD030
  list-marker-space — the number of spaces between a list
  marker and the item text, configurable per single-line
  vs multi-line and ordered vs unordered. Autofix
  normalizes the gap.
---
# List marker space rule

## Goal

Enforce a consistent gap between a list marker (`-`, `*`,
`+`, `1.`) and the item text. This closes the MD030 gap
from the
[linter comparison](../docs/background/markdown-linters.md).

## Background

markdownlint MD030 has four knobs: spaces after the marker
for single-line vs multi-paragraph items, each split by
ordered vs unordered. The default is one space everywhere.
goldmark exposes `*ast.List` and `*ast.ListItem`; the
marker run and following spaces are read from the item's
source segment.

## Design

- Rule ID: MDS061 (provisional), category `style`,
  default-enabled with one space everywhere.
- Config mirrors markdownlint:

  ```yaml
  rules:
    list-marker-space:
      ul-single: 1
      ul-multi: 1
      ol-single: 1
      ol-multi: 1
  ```

- An item is "multi" when it contains more than one block
  child (matching markdownlint's loose/tight definition).
- Autofix rewrites the inter-marker whitespace to the
  configured count; list indentation is left to MDS016.

## Tasks

1. Scaffold `internal/rules/listmarkerspace/`.
2. Implement marker/space measurement per list item.
3. Implement `rule.Configurable` for the four keys
   (replace-mode scalars; document in `ApplySettings`).
4. Implement the autofix and loop-stability test.
5. Fixture tests under the provisional
   `internal/rules/MDS061-*` directory.
6. Rule README; regenerate the docs catalog and index.
7. Add the MD030 row to the
   [linter comparison](../docs/background/markdown-linters.md).

## Acceptance Criteria

- [ ] `-  item` (two spaces) is flagged and fixed to
      `- item` under the default config.
- [ ] A multi-paragraph item honors `ul-multi` when it
      differs from `ul-single`.
- [ ] Ordered and unordered lists use their own knobs.
- [ ] Nested lists are measured per level, not flattened.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes
