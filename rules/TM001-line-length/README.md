---
id: TM001
name: line-length
description: Line exceeds maximum length.
---
# TM001: line-length

Line exceeds maximum length.

- **ID**: TM001
- **Name**: `line-length`
- **Default**: enabled, max: 80
- **Fixable**: no
- **Implementation**:
  [source](../../internal/rules/linelength/)
- **Category**: line

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max` | int | 80 | Maximum allowed line length |
| `exclude` | list | `["code-blocks", "tables", "urls"]` | Categories to exclude from checking |

Valid `exclude` values:

- `code-blocks` -- skip lines inside fenced or indented code blocks
- `tables` -- skip lines that are table rows (starting with `|`)
- `urls` -- skip lines whose only content is a URL

Set `exclude: []` to check everything (equivalent to the old `strict: true`).

The `strict` setting is deprecated. If present, `strict: true` is translated
to `exclude: []` and `strict: false` to the default exclude list.

## Config

```yaml
rules:
  line-length:
    max: 80
    exclude:
      - code-blocks
      - tables
      - urls
```

Check everything (no exclusions):

```yaml
rules:
  line-length:
    max: 80
    exclude: []
```

Skip only code blocks and URLs (check tables):

```yaml
rules:
  line-length:
    max: 120
    exclude:
      - code-blocks
      - urls
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
