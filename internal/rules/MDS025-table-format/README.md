---
id: MDS025
name: table-format
status: ready
description: Tables must have consistent column widths and padding.
---
# MDS025: table-format

Tables must have consistent column widths and padding.

- **ID**: MDS025
- **Name**: `table-format`
- **Status**: ready
- **Default**: enabled, pad: 1
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: table

## Settings

| Setting | Type | Default | Description                         |
|---------|------|---------|-------------------------------------|
| `pad`   | int  | `1`     | spaces on each side of cell content |

## Config

```yaml
rules:
  table-format:
    pad: 1
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
|--------|---------------------------|
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
|:-----|:------:|------:|
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
|--------|
| first  |
| second |
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

| Scenario                       | Behavior                                     |
|--------------------------------|----------------------------------------------|
| table inside blockquote        | `> ` prefix preserved on each line           |
| table inside list              | indentation prefix preserved                 |
| table inside fenced code block | skipped, not checked or modified             |
| escaped pipe in cell           | `\|` treated as literal, not column boundary |
| single-column table            | formatted normally with minimum width of 3   |
| inline code, links, emphasis   | display width counts raw source characters   |
