---
id: 181
title: Table structure rules
status: "🔲"
model: opus
depends-on: []
summary: >-
  New rule MDS064 (provisional) covering markdownlint
  MD055 (table pipe style), MD056 (table column count),
  and MD058 (blank lines around tables). MD056 is a
  correctness check; MD055 and MD058 are autofixable.
---
# Table structure rules

## Goal

Enforce well-formed GFM tables: consistent leading and
trailing pipes, an equal column count on every row, and
blank lines separating a table from surrounding text. This
closes the MD055 / MD056 / MD058 gap from the
[linter comparison](../docs/background/markdown-linters.md).

## Background

- MD055 table-pipe-style: leading/trailing pipe presence
  must match the configured style across all rows.
- MD056 table-column-count: every row must have the same
  cell count as the header.
- MD058 blanks-around-tables: a table needs a blank line
  before and after it.

MDS025 (table-format) already reformats cell padding and
alignment, but it does not gate pipe style, column count,
or surrounding blanks. This rule adds those checks without
duplicating MDS025's formatting pass.

## Design

- Rule ID: MDS064 (provisional), category `style`,
  default-enabled.
- Operate on goldmark GFM table nodes
  (`*east.Table` from the extension AST) and the raw rows.
- MD055: config `style` ∈ `consistent | leading_and_trailing
  | no_leading_or_trailing`; `consistent` infers from the
  first row. Autofix adds or strips edge pipes.
- MD056: flag any row whose cell count differs from the
  header; no autofix (a missing cell's content is unknown).
- MD058: flag a missing blank line on either side; autofix
  inserts it.
- Coordinate with MDS025 so a single `mdsmith fix` pass
  converges (run column/blank normalization before MDS025
  re-pads).

## Tasks

1. Scaffold `internal/rules/tablestructure/`.
2. Implement MD055, MD056, MD058 detection.
3. Implement autofix for MD055 and MD058; verify
   `mdsmith fix` is loop-stable with MDS025 enabled.
4. Implement `rule.Configurable` for the MD055 `style`.
5. Fixture tests under the provisional
   `internal/rules/MDS064-*` directory.
6. Rule README; regenerate the docs catalog and index.
7. Add the MD055 / MD056 / MD058 rows to the
   [linter comparison](../docs/background/markdown-linters.md).

## Acceptance Criteria

- [ ] A row with a missing cell is flagged (MD056), not
      auto-rewritten.
- [ ] Mixed leading/trailing pipes are flagged and
      normalized to the configured style.
- [ ] A table flush against a paragraph is flagged and a
      blank line is inserted.
- [ ] `mdsmith fix` with MDS025 also enabled converges in
      one run (no oscillation).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes
