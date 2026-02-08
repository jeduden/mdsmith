# TM004: first-line-heading

First line of the file should be a heading.

- **ID**: TM004
- **Name**: `first-line-heading`
- **Default**: enabled, level: 1
- **Fixable**: no
- **Implementation**: [`internal/rules/firstlineheading/`](../../internal/rules/firstlineheading/)

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `level` | int | 1 | Required heading level for the first line |

## Config

```yaml
rules:
  first-line-heading:
    level: 1
```

## Examples

### Bad

```markdown
Some introductory text.

# Heading
```

### Good

```markdown
# Heading

Some introductory text.
```
