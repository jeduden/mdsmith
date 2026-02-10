---
id: TM016
name: list-indent
description: List items must use consistent indentation.
---
# TM016: list-indent

List items must use consistent indentation.

- **ID**: TM016
- **Name**: `list-indent`
- **Default**: enabled, spaces: 2
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/listindent/)

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `spaces` | int | 2 | Number of spaces per indentation level |

## Config

```yaml
rules:
  list-indent:
    spaces: 2
```

## Examples

### Bad (when spaces is 2)

```markdown
- Item 1
    - Nested (4 spaces)
```

### Good (when spaces is 2)

```markdown
- Item 1
  - Nested (2 spaces)
```
