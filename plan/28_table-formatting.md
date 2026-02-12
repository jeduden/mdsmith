---
id: 28
title: Table Formatting Rule
status: 🔲
---
# Table Formatting Rule

## Goal

Implement a new rule that checks and auto-fixes markdown table
formatting to produce aligned, readable tables in plain text
view, targeting prettier-style output as the baseline.

## Tasks

### A. Design

1. Define TM021 `table-format` behaviour. A well-formatted
   table follows these rules (matching prettier output):

  - Leading and trailing pipes required on every row
  - Single space padding inside each cell
  - Each column padded to the width of its widest cell
  - Separator row dashes fill to column width
  - Alignment indicators preserved (`:---`, `:---:`,
     `---:`)
  - No trailing whitespace after the closing pipe

2. Define settings:

   | Setting | Type | Default | Description |
   |---------|------|---------|-------------|
   | `pad` | int | 1 | Spaces on each side of cell content |

   Initial implementation: single padding value applied
   uniformly.

### B. Table parsing

3. Identify table blocks by walking the goldmark AST for
   table nodes. For each table, collect the raw source
   lines.

4. Parse each row into cells by splitting on unescaped
   `|` characters. Handle escaped pipes (`\|`) inside
   cell content.

5. Compute the display width of each cell accounting
   for multi-byte characters and markdown syntax:

  - Inline code (`` ` ``) -- count only the visible text
  - Links (`[text](url)`) -- count only the link text
  - Emphasis (`*text*`, `**text**`) -- count only the
     inner text
  - Images (`![alt](url)`) -- count only the alt text

### C. Check and fix

6. Create `internal/rules/tableformat/rule.go`
   implementing `rule.Rule`, `rule.FixableRule`, and
   `rule.Configurable`.

7. Check logic: for each table compare actual cell widths
   and padding against the expected formatted output.
   Report one diagnostic per misformatted table on the
   table's first line:

  - `table is not formatted`

8. Fix logic: reformat each table:

  - Compute max display width per column
  - Rebuild each data row with padded cells
  - Rebuild separator row with correct dash count and
     preserved alignment indicators
  - Preserve content outside tables unchanged

9. Handle contextual tables:

  - Tables inside blockquotes: preserve `> ` prefix
  - Tables inside list items: preserve indentation
  - Single-column tables

10. Register rule in `init()`, add to known-rules in
    `config/load.go`, add blank import.

### D. Documentation

11. Write `rules/TM021-table-format/README.md`.

12. Create test fixtures. Bad example:

    ```markdown
    | Name | Description |
    |---|---|
    | foo | A short one |
    | barbaz | A longer description here |
    ```

    Good (formatted) example:

    ```markdown
    | Name   | Description                |
    |--------|----------------------------|
    | foo    | A short one                |
    | barbaz | A longer description here  |
    ```

### E. Tests

13. Unit tests for cell parsing: escaped pipes, empty
    cells, cells with inline code and links.

14. Unit tests for display width: ASCII, multi-byte,
    markdown syntax (links, emphasis, code).

15. Unit tests for check: detect misaligned tables,
    missing padding, short separator dashes.

16. Unit tests for fix: verify prettier-style output for
    various table shapes (alignment indicators, single
    column, wide content, empty cells).

17. Unit tests for contextual tables: blockquote prefix,
    list indent, nested blockquote.

18. Integration tests with bad/good/fixed fixtures.

## Acceptance Criteria

- [ ] TM021 detects tables not matching prettier format
- [ ] Fix produces prettier-style aligned tables
- [ ] Alignment indicators (`:---:`, `---:`) preserved
- [ ] Escaped pipes in cell content handled correctly
- [ ] Display width accounts for links, code, emphasis
- [ ] Tables inside blockquotes and lists handled
- [ ] Single diagnostic per misformatted table
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
