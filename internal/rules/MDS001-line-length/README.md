---
id: MDS001
name: line-length
description: Line exceeds maximum length.
---
# MDS001: line-length

Line exceeds maximum length.

- **ID**: MDS001
- **Name**: `line-length`
- **Default**: enabled, max: 80
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: line

## Settings

| Setting        | Type | Default                           | Description                                              |
|----------------|------|-----------------------------------|----------------------------------------------------------|
| `max`            | int  | 80                                | Maximum allowed line length                              |
| `heading-max`    | int  | --                                | Max length for heading lines; inherits `max` when unset    |
| `code-block-max` | int  | --                                | Max length for code block lines; inherits `max` when unset |
| `stern`          | bool | false                             | Only flag long lines that contain a space past the limit |
| `exclude`        | list | `["code-blocks", "tables", "urls"]` | Categories to exclude from checking                      |

Valid `exclude` values:

- `code-blocks` -- skip lines inside fenced or indented code blocks
- `tables` -- skip lines that are table rows (starting with `|`)
- `urls` -- skip lines whose only content is a URL

Set `exclude: []` to check everything (equivalent to the old `strict: true`).

The `strict` setting is deprecated. If present, `strict: true` is translated
to `exclude: []` and `strict: false` to the default exclude list.

### Per-category limits

When `heading-max` is set, heading lines (ATX and Setext) use that
limit instead of `max`. When `code-block-max` is set, lines inside fenced or
indented code blocks use that limit instead of `max`. When either setting is
unset (absent), lines of that type inherit `max`.

Per-category limits and `exclude` compose.
A code block line excluded via `exclude: [code-blocks]` is still skipped,
even when `code-block-max` is set.

### Stern mode

When `stern: true`, a line that exceeds the active limit is flagged only if it
contains a space character at or beyond the limit column. Lines with no space
past the limit (for example a long URL at the end of a line) are allowed.

Stern mode applies independently of `exclude`. A line inside a code block that
is excluded via `exclude: [code-blocks]` is still skipped regardless of stern.
Stern uses the active max for each line type, so it respects `heading-max` and
`code-block-max` when set.

## Config

Enable (default):

```yaml
rules:
  line-length:
    max: 80
    exclude:
      - code-blocks
      - tables
      - urls
```

Disable:

```yaml
rules:
  line-length: false
```

Custom (per-category limits):

```yaml
rules:
  line-length:
    max: 80
    heading-max: 100
    code-block-max: 120
    exclude:
      - tables
      - urls
```

Custom (stern mode; allow long lines without spaces past the limit):

```yaml
rules:
  line-length:
    max: 80
    stern: true
```

Custom (check everything; no exclusions):

```yaml
rules:
  line-length:
    max: 80
    exclude: []
```

Custom (skip only code blocks and URLs; check tables):

```yaml
rules:
  line-length:
    max: 120
    exclude:
      - code-blocks
      - urls
```

## Examples

### Bad

Line exceeds the default 80-character limit:

```markdown
This is a very long line that exceeds the maximum allowed length of eighty characters and should trigger a lint warning.
```

Heading exceeds `heading-max: 60`:

```markdown
# This heading is deliberately made long enough to exceed sixty chars
```

Long line with spaces past the limit (`stern: true` still flags it):

```markdown
This line has words that continue well past the eighty character column and keep going on.
```

### Good

```markdown
This line is within the limit.
```

Heading within `heading-max: 100` (even though it exceeds `max: 80`):

```markdown
# This heading is about ninety characters and fits within the heading-max of one hundred
```

Long URL without spaces past the limit (`stern: true` allows it):

```markdown
https://example.com/this-is-a-very-long-url-that-exceeds-eighty-characters-but-has-no-spaces
```
