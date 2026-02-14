---
id: MDS016
name: list-indent
description: List items must use consistent indentation.
---
# MDS016: list-indent

List items must use consistent indentation.

- **ID**: MDS016
- **Name**: `list-indent`
- **Default**: enabled, spaces: 2
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: list

## Settings

| Setting | Type | Default | Description                            |
|---------|------|---------|----------------------------------------|
| `spaces`  | int  | 2       | Number of spaces per indentation level |

## Config

Enable (default):

```yaml
rules:
  list-indent:
    spaces: 2
```

Disable:

```yaml
rules:
  list-indent: false
```

Custom (4-space list indent):

```yaml
rules:
  list-indent:
    spaces: 4
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
