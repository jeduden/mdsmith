---
id: 181
title: Table structure rules
status: "✅"
model: opus
depends-on: []
summary: >-
  Fold the GFM well-formedness checks — MD055 table pipe
  style, MD056 column count, MD058 surrounding blank lines —
  into MDS025 (table-format) so one rule owns table parsing,
  structure, and alignment.
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

MDS025 (`table-format`) already owned the prettier-style
alignment pass. Folding the structure checks into the same
rule gives users one `table-format` setting block. One
`Rule.Fix` call now runs structure normalisation and
alignment on the same buffer. The fix engine still loops
fixable rules to stability. MDS025 itself, however, has no
second rule to oscillate against.

## Design

- Rule ID: MDS025 (`table-format`), category `table`, nature
  `style`, default-enabled.
- Two code paths inside one rule:
  - The line-based GFM table parser in
    `internal/rules/tableformat/structure.go` handles header
    + delimiter + body rows with edge pipes optional. It
    powers the MD055/056/058 checks and the structural fix
    (edge normalisation, blank-line insertion).
  - The existing `tablefmt` package continues to own the
    prettier-style alignment of bordered tables and is run
    after the structure fix on the same Fix call so column
    widths re-pad in one pass.
- Row prefix detection: a `>` blockquote-marker chain (or
  list indentation) shared by every row. Blockquoted and
  indented tables are linted; the MD058 blank line inside a
  blockquote is the bare `>` marker, not an empty line.
- Skip set: both passes ignore fenced/indented code,
  processing-instruction blocks, and generated-section
  bodies. `formatSkipLines` builds the union for the
  alignment pass so it matches the structure pass.
- `style` setting ∈ `consistent | leading_and_trailing |
  no_leading_or_trailing`; `consistent` infers from each
  table's header row. Default: `consistent`. Autofix adds
  or strips edge pipes.
- MD056: flag any row whose cell count differs from the
  header; the structure pass never auto-rewrites. The
  alignment pass, however, pads short rows with empty cells
  while reformatting widths, so a fixed file is structurally
  clean even when the original missed a cell.
- MD058: flag a missing blank line on either side; autofix
  inserts it.

## Tasks

1. [x] Port the GFM parser and MD055/056/058 logic into
   `internal/rules/tableformat/structure.go`.
2. [x] Extend `tableformat.Rule` with a `style` setting and
   chain the structure fix before the alignment fix on the
   same Fix call.
3. [x] Bring the alignment pass's skip set to parity with
   the structure pass (PI blocks + generated ranges, not
   just code blocks).
4. [x] Migrate the structure fixtures into
   `internal/rules/MDS025-table-format/{good,bad,fixed}/`
   with merged-rule diagnostic lists.
5. [x] Update `internal/rules/MDS025-table-format/README.md`
   for the merged scope (style setting, MD055/056/058 in
   markdownlint frontmatter, structural examples and edge
   cases).
6. [x] Update `docs/research/markdownlint-coverage/README.md`
   and `docs/background/markdown-linters.md` to point at
   MDS025 for MD055/056/058.
7. [x] Delete the standalone `tablestructure` package and
   the `MDS060-table-structure` fixture directory; drop the
   import in `internal/rules/all/all.go` and the integration
   test.
8. [x] Regenerate `internal/rules/index.md` via `mdsmith
   fix`.

## Acceptance Criteria

- [x] A row with a missing cell is flagged (MD056); the
      structure pass leaves it alone, the alignment pass
      pads it.
- [x] Mixed leading/trailing pipes are flagged and
      normalised to the configured style.
- [x] A table flush against a paragraph is flagged and a
      blank line is inserted.
- [x] `mdsmith fix` converges in one run.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.
- [x] `mdsmith check .` passes.
