---
id: TM003
name: heading-increment
description: Heading levels should increment by one. No jumping from `#` to `###`.
---
# TM003: heading-increment

Heading levels should increment by one. No jumping from `#` to `###`.

- **ID**: TM003
- **Name**: `heading-increment`
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](../../internal/rules/headingincrement/)

## Config

```yaml
rules:
  heading-increment: true
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
