---
id: 181
title: Table structure rules
status: "✅"
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

- Rule ID: MDS060, category `table`, nature `style`,
  default-enabled. (MDS064, the provisional id in the
  original plan, shipped first as atx-heading-whitespace;
  MDS060 was the lowest free, unreserved id. The rule-readme
  schema makes `style` a `nature`, not a `category`.)
- Line-based GFM table detection (header + delimiter + body
  rows; edge pipes optional), not `*east.Table`. The
  extension AST and the MDS025 `tablefmt` parser both require
  edge pipes on every row, so they cannot see the borderless
  and mixed-pipe tables MD055 must flag.
- Row prefix detection mirrors MDS025's `tablefmt`: a `>`
  blockquote-marker chain (or list indentation) shared by
  every row. Blockquoted and indented tables are linted; the
  MD058 blank line inside a blockquote is the bare `>` marker,
  not an empty line.
- MD055: config `style` ∈ `consistent | leading_and_trailing
  | no_leading_or_trailing`; `consistent` infers from the
  header row. Autofix adds or strips edge pipes.
- MD056: flag any row whose cell count differs from the
  header; no autofix (a missing cell's content is unknown).
- MD058: flag a missing blank line on either side; autofix
  inserts it.
- Coordinate with MDS025 so a single `mdsmith fix` pass
  converges (run column/blank normalization before MDS025
  re-pads).

## Tasks

1. [x] Scaffold `internal/rules/tablestructure/`.
2. [x] Implement MD055, MD056, MD058 detection.
3. [x] Implement autofix for MD055 and MD058; verify
   `mdsmith fix` is loop-stable with MDS025 enabled.
4. [x] Implement `rule.Configurable` for the MD055 `style`.
5. [x] Fixture tests under the
   `internal/rules/MDS060-table-structure` directory.
6. [x] Rule README; regenerate the docs catalog and index.
7. [x] Add the MD055 / MD056 / MD058 rows to the
   [linter comparison](../docs/background/markdown-linters.md).

## Acceptance Criteria

- [x] A row with a missing cell is flagged (MD056), not
      auto-rewritten.
- [x] Mixed leading/trailing pipes are flagged and
      normalized to the configured style.
- [x] A table flush against a paragraph is flagged and a
      blank line is inserted.
- [x] `mdsmith fix` with MDS025 also enabled converges in
      one run (no oscillation).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] `mdsmith check .` passes
