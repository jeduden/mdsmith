---
id: TM022
name: max-file-length
description: File must not exceed maximum number of lines.
---
# TM022: max-file-length

File must not exceed maximum number of lines.

- **ID**: TM022
- **Name**: `max-file-length`
- **Default**: enabled, max: 300
- **Fixable**: no
- **Implementation**:
  [source](../../internal/rules/maxfilelength/)
- **Category**: meta

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max` | int | 300 | Maximum lines allowed |

## Config

```yaml
rules:
  max-file-length:
    max: 500
```

Disable:

```yaml
rules:
  max-file-length: false
```

## Examples

### Good

A file with 300 or fewer lines produces no diagnostic.

### Bad

A file with more than 300 lines triggers:

```text
file.md:1:1 TM022 file too long (350 > 300)
```
