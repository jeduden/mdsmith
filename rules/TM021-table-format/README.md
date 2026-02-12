---
id: TM021
name: table-format
description: Tables must have consistent column widths and padding.
---
# TM021: table-format

Tables must have consistent column widths and padding.

- **ID**: TM021
- **Name**: `table-format`
- **Default**: enabled, pad: 1
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/tableformat/)
- **Category**: table

## Settings

| Setting | Type | Default | Description                         |
|---------|------|---------|-------------------------------------|
| `pad`     | int  | `1`       | spaces on each side of cell content |

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

```markdown
| Name   | Description               |
|--------|---------------------------|
| foo    | A short one               |
| barbaz | A longer description here |
```

### Bad

```markdown
| Name | Description |
|---|---|
| foo | A short one |
| barbaz | A longer description here |
```

### Good -- alignment indicators

```markdown
| Left | Center | Right |
|:-----|:------:|------:|
| aaa  | bbb    | ccc   |
```

## Edge Cases

| Scenario                       | Behavior                                   |
|--------------------------------|--------------------------------------------|
| table inside blockquote        | `> ` prefix preserved on each line           |
| table inside list              | indentation prefix preserved               |
| table inside fenced code block | skipped, not checked or modified           |
| escaped pipe in cell           | `\|` treated as literal, not column boundary |
| single-column table            | formatted normally with minimum width of 3 |
| inline code, links, emphasis   | display width counts only visible text     |
