---
id: TM019
name: catalog
description: Catalog content must reflect selected front matter fields from files matching its glob.
---
# TM019: catalog

Catalog content must reflect selected front matter fields
from files matching its glob.

- **ID**: TM019
- **Name**: `catalog`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/catalog/)
- **Category**: meta
- **Archetype**:
  [generated-section](../../archetypes/generated-section/)

## Directive: `catalog`

Lists files matching a glob pattern. Uses *template mode*
when the YAML body has a `row` key, and *minimal mode*
otherwise.

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `glob` | yes | -- | Relative file glob |
| `sort` | no | `path` | Sort key |

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `columns` | no | -- | Column width/wrapping |

The `glob` supports `*`, `?`, `[...]`, and `**`. It does
not allow absolute paths or `..` traversal.

### Template fields

The `row` section uses Go `text/template` syntax.

- `{{.filename}}` -- relative path from the marker file's
  directory (no leading `./`).
- Other names (e.g., `{{.title}}`) -- looked up in the
  matched file's YAML front matter.
- Missing field -> empty string.
- Non-string value -> Go default string format.

### Column constraints

The `columns` parameter sets per-column width limits. Each
key is a template field name. Options:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `max-width` | int | -- | Max character width. |
| `wrap` | string | `truncate` | `truncate` or `br`. |

Links and inline code are not split mid-span.

```yaml
columns:
  description:
    max-width: 50
    wrap: br
```

### Sort behavior

Format: `[-]KEY`. A `-` prefix means descending order.

Built-in keys: `path` (default), `filename`. Any other
key is looked up in front matter. Missing values sort as
empty string. Sorting ignores case; ties break by path.

### Minimal mode

Without `row`, `header`, or `footer`, the directive outputs
a bullet list: `- [<basename>](<relative-path>)`. Front
matter is only read when the sort key needs it.

### Rendering logic

1. Files matched: `header` + rows + `footer` (`empty` ignored)
2. No files, `empty` defined: `empty` text
3. No files, no `empty`: zero lines between markers

See the
[archetype docs](../../archetypes/generated-section/)
for newline handling and chomp details.

## Config

```yaml
rules:
  catalog: true
```

Disable:

```yaml
rules:
  catalog: false
```

## Examples

### Good -- minimal

```markdown
<!-- catalog
glob: docs/*.md
-->
- [api-reference.md](docs/api-reference.md)
- [getting-started.md](docs/getting-started.md)
<!-- /catalog -->
```

### Good -- template with header

```markdown
<!-- catalog
glob: docs/*.md
header: |
  | Title | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
-->
| Title | Description |
|-------|-------------|
| [API Reference](docs/api-reference.md) | Complete API documentation |
<!-- /catalog -->
```

### Good -- empty fallback

```markdown
<!-- catalog
glob: docs/*.md
empty: No documents found.
-->
No documents found.
<!-- /catalog -->
```

### Good -- descending sort

```markdown
<!-- catalog
glob: docs/*.md
sort: -title
row: "- [{{.title}}]({{.filename}})"
-->
- [Getting Started](docs/getting-started.md)
- [API Reference](docs/api-reference.md)
<!-- /catalog -->
```

### Bad -- stale content

```markdown
<!-- catalog
glob: docs/*.md
-->
- [api-reference.md](docs/api-reference.md)
<!-- /catalog -->
```

Diagnostic: `generated section is out of date`

### Bad -- unclosed marker

```markdown
<!-- catalog
glob: docs/*.md
-->
- [api-reference.md](docs/api-reference.md)
```

Diagnostic: `generated section has no closing marker`

### Bad -- missing `row` with `header`

```markdown
<!-- catalog
glob: docs/*.md
header: |
  | Title | Description |
  |-------|-------------|
-->
<!-- /catalog -->
```

Diagnostic:
`generated section template missing required "row" key`

## Diagnostics

| Condition | Message |
|-----------|---------|
| Missing `glob` | `...missing required "glob"` |
| Empty `glob` | `...has empty "glob"` |
| Absolute glob | `...has absolute glob path` |

| Condition | Message |
|-----------|---------|
| Glob with `..` | `...".." path traversal` |
| Invalid glob | `...invalid glob pattern` |
| Empty sort | `...empty "sort" value` |
| Invalid sort | `...invalid sort value` |

All messages above are prefixed with
`generated section directive`. Column is always 1.

See the
[archetype documentation](../../archetypes/generated-section/)
for shared diagnostics (content mismatch, unclosed markers,
nested markers, YAML errors, template errors).

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| No front matter | Others -> empty |
| Invalid front matter | Treated as absent |
| Missing field | Empty string |
| Unreadable file | Skipped |

| Scenario | Behavior |
|----------|----------|
| Glob matches directory | Skipped |
| Glob matches linted file | Included |
| Binary file | Included; no front matter |
| Symlinks | Followed |

| Scenario | Behavior |
|----------|----------|
| Dotfiles | Matched by `*`/`**` |
| Absolute/`..` glob | Diagnostic |
| Brace expansion | Supported |
| Empty glob/sort | Diagnostic |

| Scenario | Behavior |
|----------|----------|
| `sort: "-"` | Diagnostic |
| Sort with whitespace | Diagnostic |
| No files matched | `empty` fallback |
| Files + `empty` | `empty` ignored |

See the
[archetype documentation](../../archetypes/generated-section/)
for shared edge cases (markers in code blocks, multiple marker
pairs, line endings, template errors).
