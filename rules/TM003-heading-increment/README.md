# TM003: heading-increment

Heading levels should increment by one. No jumping from `#` to `###`.

- **ID**: TM003
- **Name**: `heading-increment`
- **Default**: enabled
- **Fixable**: no
- **Implementation**: [`internal/rules/headingincrement/`](../../internal/rules/headingincrement/)

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
