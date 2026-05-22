---
id: MDS025
name: table-format
status: ready
description: Tables must have consistent edge pipes, equal column counts, surrounding blank lines, and prettier-style alignment.
category: table
nature: style
maintainability: null
markdownlint:
  - id: MD055
    name: table-pipe-style
  - id: MD056
    name: table-column-count
    partial: true
  - id: MD058
    name: blanks-around-tables
---
# MDS025: table-format

Tables must have consistent edge pipes, equal column counts,
surrounding blank lines, and prettier-style alignment.

A GFM table must satisfy four conditions:

1. Every row's leading/trailing pipe presence matches the configured
   `style` (the markdownlint MD055 check).
2. Every non-header row, including the delimiter row, has the same
   logical cell count as the header (MD056).
3. The table has a blank line before and after (MD058).
4. Each bordered table has aligned column widths and consistent
   padding (the prettier-style alignment pass that gave the rule its
   name), with the configured `separator-style`.

Conditions 1, 3, and 4 are auto-fixed by `mdsmith fix`. Condition 2
(MD056) is flagged but never auto-rewritten on its own: a missing
cell's intended content is unknown. The alignment pass does pad
short rows with empty cells while it normalises widths, so a fixed
*bordered* table is structurally clean even when the original
missed a cell. A borderless table is not formatted by the
alignment pass — tablefmt requires edge pipes on every row. MD056
therefore keeps firing on a borderless short row after fix; add
the edge pipes or fill the cell to resolve it.

## Settings

| Setting           | Type   | Default        | Description                                                                        |
|-------------------|--------|----------------|------------------------------------------------------------------------------------|
| `pad`             | int    | `1`            | spaces on each side of cell content                                                |
| `separator-style` | string | `"spaced"`     | separator row layout (see examples below): `spaced` or `compact`                   |
| `style`           | string | `"consistent"` | edge-pipe style: `consistent`, `leading_and_trailing`, or `no_leading_or_trailing` |

`consistent` infers the required edge-pipe shape from each table's
header row and holds every other row to it. The other two values
require or forbid leading and trailing pipes on every row.

Separator row examples:

- `spaced` (the GFM-spec form):

  ```text
  | --- |
  ```

- `compact` (dense form):

  ```text
  |---|
  ```

## Config

```yaml
rules:
  table-format:
    pad: 1
    separator-style: spaced
    style: consistent
```

Opt into the dense compact form:

```yaml
rules:
  table-format:
    separator-style: compact
```

Disable:

```yaml
rules:
  table-format: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Formatted Table

| Name   | Description               |
| ------ | ------------------------- |
| foo    | A short one               |
| barbaz | A longer description here |
```

<?/include?>

### Good -- alignment indicators

<?include
file: good/alignment.md
wrap: markdown
?>

```markdown
# Aligned Table

| Left | Center | Right |
| :--- | :----: | ----: |
| aaa  | bbb    | ccc   |
```

<?/include?>

### Good -- single column

<?include
file: good/single-column.md
wrap: markdown
?>

```markdown
# Single Column

| Item   |
| ------ |
| first  |
| second |
```

<?/include?>

### Good -- compact (opt-in)

With `separator-style: compact` the same column widths apply; only the
separator row collapses its padding spaces. The fixture below carries
the setting in front matter so the integration tests exercise the
compact path.

<?include
file: good/compact.md
wrap: markdown
?>

```markdown
# Formatted Compact Table

| Name   | Description               |
|--------|---------------------------|
| foo    | A short one               |
| barbaz | A longer description here |
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

### Good -- blockquoted

<?include
file: good/blockquote.md
wrap: markdown
?>

```markdown
# Blockquote table

> Quoted intro.
>
> | Key | Value |
> | --- | ----- |
> | a   | one   |
>
> Quoted outro.
```

<?/include?>

### Bad -- misaligned

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Misaligned Table

| Name | Description |
|---|---|
| foo | A short one |
| barbaz | A longer description here |
```

<?/include?>

### Bad -- alignment indicators

<?include
file: bad/alignment.md
wrap: markdown
?>

```markdown
# Misaligned Alignment

| Left | Center | Right |
|:---|:---:|---:|
| aaa | bbb | ccc |
```

<?/include?>

### Bad -- mixed edge pipes (MD055)

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

### Bad -- missing cell (MD056)

<?include
file: bad/missing-cell.md
wrap: markdown
?>

```markdown
# Missing cell

| Key | Value |
| --- | ----- |
| a   | one   |
| b   |
```

<?/include?>

### Bad -- no surrounding blanks (MD058)

<?include
file: bad/no-blank-lines.md
wrap: markdown
?>

```markdown
# No blanks

Paragraph before.
| Key | Value |
| --- | ----- |
| a   | one   |
Paragraph after.
```

<?/include?>

## Edge Cases

| Scenario                            | Behavior                                                                              |
|-------------------------------------|---------------------------------------------------------------------------------------|
| table inside blockquote             | `> ` prefix preserved on each line; MD058 blanks use the bare `>` marker              |
| table inside list                   | indentation prefix preserved                                                          |
| table inside fenced code block      | skipped, not checked or modified                                                      |
| table inside a generated section    | skipped (both passes); the directive owns the bytes                                   |
| escaped pipe in cell                | `\|` treated as literal, not a column boundary                                        |
| single-column table                 | formatted normally with minimum width of 3                                            |
| inline code, links, emphasis        | width measured in display columns, syntax included                                    |
| short row (MD056)                   | flagged; the alignment pass pads it with empty cells while reformatting widths        |
| `no_leading_or_trailing` + bordered | edges stripped; alignment pass then leaves the now-borderless table alone (converges) |

## Meta-Information

- **ID**: MDS025
- **Name**: `table-format`
- **Status**: ready
- **Default**: enabled, pad: 1, separator-style: spaced, style: consistent
- **Fixable**: yes (MD055 edges, MD058 blanks, alignment; MD056 column count is flagged only)
- **Implementation**:
  [source](./)
- **Category**: table
- **markdownlint coverage**: MD055 (table-pipe-style), MD056
  (table-column-count, flag only), MD058 (blanks-around-tables)
