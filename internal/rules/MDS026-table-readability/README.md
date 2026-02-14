---
id: MDS026
name: table-readability
description: Tables must stay within readability complexity limits.
---
# MDS026: table-readability

Tables must stay within readability complexity limits.

- **ID**: MDS026
- **Name**: `table-readability`
- **Default**: enabled
  - `max-columns`: `8`
  - `max-rows`: `30`
  - `max-words-per-cell`: `30`
  - `max-column-width-variance`: `60.0`
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: table

This rule is separate from MDS023 and MDS024.
Table complexity needs table-specific metrics.

## Settings

| Setting                   | Type  | Default | Description                                                      |
|---------------------------|-------|---------|------------------------------------------------------------------|
| `max-columns`               | int   | `8`       | Maximum columns per table                                        |
| `max-rows`                  | int   | `30`      | Maximum data rows per table                                      |
| `max-words-per-cell`        | int   | `30`      | Maximum words allowed in a single cell                           |
| `max-column-width-variance` | float | `60.0`    | Maximum ratio between widest and narrowest average column widths |

## Config

```yaml
rules:
  table-readability:
    max-columns: 8
    max-rows: 30
    max-words-per-cell: 30
    max-column-width-variance: 60.0
```

Disable:

```yaml
rules:
  table-readability: false
```

## Examples

### Good

```markdown
| Metric | Target |
|--------|--------|
| Latency | <200ms |
| Error rate | <1% |
```

### Bad

```markdown
| A | B | C | D | E | F | G |
|---|---|---|---|---|---|---|
| 1 | 2 | 3 | 4 | 5 | 6 | 7 |
```

## Diagnostics

- `table has too many columns (N > limit)`
- `table has too many rows (N > limit)`
- `table cell has too many words (N > limit)`
- `table has high column width variance (ratio > limit)`

## Edge Cases

- Tables inside fenced code blocks are skipped.
- Blockquote and indented tables are checked.
