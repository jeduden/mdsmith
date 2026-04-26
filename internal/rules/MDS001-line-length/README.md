---
id: MDS001
name: line-length
status: ready
description: Line exceeds maximum length.
---
# MDS001: line-length

Line exceeds maximum length.

## Settings

| Setting          | Type | Default                             | Description                                                |
|------------------|------|-------------------------------------|------------------------------------------------------------|
| `max`            | int  | 80                                  | Maximum allowed line length                                |
| `heading-max`    | int  | --                                  | Max length for heading lines; inherits `max` when unset    |
| `code-block-max` | int  | --                                  | Max length for code block lines; inherits `max` when unset |
| `stern`          | bool | false                               | Only flag long lines that contain a space past the limit   |
| `exclude`        | list | `["code-blocks", "tables", "urls"]` | Categories to exclude from checking                        |

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

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

This line is deliberately made to exceed the eighty character limit by adding extra words here now.
```

<?/include?>

Line exceeds a custom `max: 120` limit:

<?include
file: bad/max-120.md
wrap: markdown
?>

```markdown
# Title

This line is deliberately made very long so that it exceeds the one hundred and twenty character limit set right here!!!!
```

<?/include?>

Heading exceeds `heading-max: 60`:

<?include
file: bad/heading-over-limit.md
wrap: markdown
?>

```markdown
# This heading is deliberately made long enough to exceed sixty chars
```

<?/include?>

Long line with spaces past the limit (`stern: true` still flags it):

<?include
file: bad/stern-spaces-past-limit.md
wrap: markdown
?>

```markdown
# Title

This line is deliberately made to exceed the eighty character limit by adding extra words here now.
```

<?/include?>

No exclusions — URLs and code blocks are also checked:

<?include
file: bad/no-exclusions.md
wrap: markdown
?>

```markdown
# Title

This line is deliberately made to exceed the eighty character limit by adding extra words here now.

https://example.com/this-is-a-very-long-url-that-exceeds-eighty-characters-and-should-be-flagged
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

This line is well within the eighty character limit.
```

<?/include?>

Heading within `heading-max: 100` (even though it exceeds `max: 80`):

<?include
file: good/heading-within-limit.md
wrap: markdown
?>

```markdown
# This heading is within the heading-max limit of one hundred characters
```

<?/include?>

Code block within `code-block-max: 120`:

<?include
file: good/code-block-within-limit.md
wrap: markdown
?>

````markdown
# Title

```text
This line inside a code block is over 80 characters but within the code-block-max limit of one hundred and twenty.
```
````

<?/include?>

## Meta-Information

- **ID**: MDS001
- **Name**: `line-length`
- **Status**: ready
- **Default**: enabled, max: 80
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: line
