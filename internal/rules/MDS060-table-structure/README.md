---
id: MDS060
name: table-structure
status: ready
description: >-
  Table rows must use the configured leading and trailing pipe
  style. Every row must match the header's cell count. A blank
  line must sit before and after each table.
nature: style
category: table
maintainability: null
markdownlint:
  - id: MD055
    name: table-pipe-style
  - id: MD056
    name: table-column-count
  - id: MD058
    name: blanks-around-tables
---
# MDS060: table-structure

Table rows must use the configured leading and trailing pipe
style. Every row must match the header's cell count. A blank
line must sit before and after each table.

Three checks on GFM pipe tables, complementing MDS025
(table-format), which owns cell padding and alignment:

- **Pipe style** (markdownlint MD055): every row must agree
  with the configured edge-pipe style. Auto-fixed.
- **Column count** (markdownlint MD056): every row must have
  the same cell count as the header. Flagged only — a missing
  cell's content is unknown, so the row is never rewritten.
- **Surrounding blanks** (markdownlint MD058): a table needs a
  blank line before and after it. Auto-fixed by inserting the
  blank line.

Blockquoted (`> | ... |`) and list-indented tables are
checked too; the shared `>`/indent prefix is recognized the
same way MDS025 recognizes it. Tables inside fenced code
blocks, processing-instruction blocks, and generated
`<?include?>` / `<?catalog?>` bodies are left alone.

## Settings

| Setting | Type   | Default      | Description              |
|---------|--------|--------------|--------------------------|
| `style` | string | `consistent` | Required edge-pipe style |

`style` accepts:

- `consistent` — infer the required edge-pipe shape from the
  header row and hold every other row to it.
- `leading_and_trailing` — require a leading and a trailing
  pipe on every row.
- `no_leading_or_trailing` — forbid leading and trailing pipes
  on every row.

Keep the default `consistent` when MDS025 (table-format) is
on. MDS025 emits leading and trailing pipes. A fixed bordered
table then satisfies `consistent`, so one `mdsmith fix` run
converges. `no_leading_or_trailing` is also stable. MDS060
strips the edge pipes. MDS025 formats only bordered tables,
so it then ignores that table. The tradeoff is a loss of
MDS025 column alignment, not oscillation.

## Config

Enable with default settings:

```yaml
rules:
  table-structure: true
```

Disable:

```yaml
rules:
  table-structure: false
```

Require bare rows (no edge pipes):

```yaml
rules:
  table-structure:
    style: no_leading_or_trailing
```

## Examples

### Good -- bordered

<?include
file: good/bordered.md
wrap: markdown
?>

```markdown
# Bordered table

A leading and trailing pipe on every row, equal column
counts, and a blank line on each side.

| Key | Value |
|-----|-------|
| a   | one   |
| b   | two   |

End of section.
```

<?/include?>

### Good -- borderless

<?include
file: good/borderless.md
wrap: markdown
?>

```markdown
# Borderless table

No leading or trailing pipe on any row. The style is
consistent, so the table passes.

Key | Value
--- | -----
a | one
b | two
```

<?/include?>

### Good -- blockquote

<?include
file: good/blockquote.md
wrap: markdown
?>

```markdown
# Blockquote table

> Quoted intro.
>
> | Key | Value |
> |-----|-------|
> | a   | one   |
>
> Quoted outro.
```

<?/include?>

### Bad -- mixed pipes

<?include
file: bad/mixed-pipes.md
wrap: markdown
?>

```markdown
# Mixed pipes

Key | Value
--- | -----
| a | one |
b | two
```

<?/include?>

### Bad -- missing cell

<?include
file: bad/missing-cell.md
wrap: markdown
?>

```markdown
# Missing cell

| Key | Value |
|-----|-------|
| a   | one   |
| b   |
```

<?/include?>

### Bad -- no surrounding blanks

<?include
file: bad/no-blank-lines.md
wrap: markdown
?>

```markdown
# No blanks

Paragraph before.
| Key | Value |
|-----|-------|
| a   | one   |
Paragraph after.
```

<?/include?>

## Diagnostics

- `table pipe style; expected <shape>` — a row's edge pipes do
  not match the style.
- `table column count; expected N, got M` — a row's cell count
  differs from the header.
- `missing blank line before table` /
  `missing blank line after table` — the table is flush
  against adjacent content.

## Meta-Information

- **ID**: MDS060
- **Name**: `table-structure`
- **Status**: ready
- **Default**: enabled
- **Fixable**: MD055 and MD058 only; MD056 is flagged, not fixed
- **Implementation**:
  [source](./)
- **Category**: table
