# TM001: line-length

Line exceeds maximum length.

- **ID**: TM001
- **Name**: `line-length`
- **Default**: enabled, max: 80
- **Fixable**: no
- **Implementation**: [`internal/rules/linelength/`](../../internal/rules/linelength/)

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max` | int | 80 | Maximum allowed line length |
| `strict` | bool | false | When false, skips lines containing only a URL or inside code blocks |

## Config

```yaml
rules:
  line-length:
    max: 80
    strict: false
```

Disable:

```yaml
rules:
  line-length: false
```

## Examples

### Bad

```markdown
This is a very long line that exceeds the maximum allowed length of eighty characters and should trigger a lint warning.
```

### Good

```markdown
This line is within the limit.
```
