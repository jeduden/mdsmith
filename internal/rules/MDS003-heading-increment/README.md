---
id: MDS003
name: heading-increment
description: Heading levels should increment by one. No jumping from `#` to `###`.
---
# MDS003: heading-increment

Heading levels should increment by one. No jumping from `#` to `###`.

- **ID**: MDS003
- **Name**: `heading-increment`
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

## Config

Enable:

```yaml
rules:
  heading-increment: true
```

Disable:

```yaml
rules:
  heading-increment: false
```

## Examples

### Bad

```markdown
# Heading 1

### Heading 3
```

### Good

```markdown
# Heading 1

## Heading 2

### Heading 3
```
