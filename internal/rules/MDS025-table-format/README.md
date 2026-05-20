---
id: MDS025
name: table-format
status: ready
description: Tables must have consistent column widths and padding.
category: table
nature: style
maintainability: null
markdownlint: null
---
# MDS025: table-format

Tables must have consistent column widths and padding.

## Settings

| Setting           | Type   | Default    | Description                                                  |
|-------------------|--------|------------|--------------------------------------------------------------|
| `pad`             | int    | `1`        | spaces on each side of cell content                          |
| `separator-style` | string | `"spaced"` | separator row layout (see examples below): spaced or compact |

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

### Bad

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

### Bad -- alignment

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

## Edge Cases

| Scenario                       | Behavior                                           |
|--------------------------------|----------------------------------------------------|
| table inside blockquote        | `> ` prefix preserved on each line                 |
| table inside list              | indentation prefix preserved                       |
| table inside fenced code block | skipped, not checked or modified                   |
| escaped pipe in cell           | `\|` treated as literal, not column boundary       |
| single-column table            | formatted normally with minimum width of 3         |
| inline code, links, emphasis   | width measured in display columns, syntax included |

## Meta-Information

- **ID**: MDS025
- **Name**: `table-format`
- **Status**: ready
- **Default**: enabled, pad: 1
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: table
