---
id: MDS019
name: catalog
status: ready
description: Catalog content must reflect selected front matter fields from files matching its glob.
---
# MDS019: catalog

Catalog content must reflect selected front matter fields
from files matching its glob.

- **ID**: MDS019
- **Name**: `catalog`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: meta
- **Archetype**:
  [generated-section](../../../docs/design/archetypes/generated-section/)

## Directive: `catalog`

Lists files matching a glob pattern. Uses *template mode*
when the YAML body has a `row` key, and *minimal mode*
otherwise.

### Parameters

| Parameter | Required | Default | Description        |
|-----------|----------|---------|--------------------|
| `glob`    | yes      | --      | Relative file glob |
| `sort`    | no       | `path`  | Sort key           |

| Parameter | Required | Default | Description           |
|-----------|----------|---------|-----------------------|
| `columns` | no       | --      | Column width/wrapping |

The `glob` accepts a single string or a YAML list of
strings. It supports `*`, `?`, `[...]`, `**`, and `{a,b}`
brace expansion. It does not allow absolute paths or `..`
traversal.

Single pattern:

```yaml
glob: "docs/**/*.md"
```

Multiple patterns (YAML list):

```yaml
glob:
  - "docs/**/*.md"
  - "plan/*.md"
```

Brace expansion:

```yaml
glob: "internal/rules/{MDS001,MDS002}*/README.md"
```

When multiple patterns are provided, files are collected
from all patterns (deduplicated), then sorted together.

Do not use YAML folded scalars (`>`, `>-`) in the YAML
body. See the
[archetype docs](../../../docs/design/archetypes/generated-section/)
for details.

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

| Option      | Type   | Default    | Description          |
|-------------|--------|------------|----------------------|
| `max-width` | int    | --         | Max character width. |
| `wrap`      | string | `truncate` | `truncate` or `br`.  |

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
[archetype docs](../../../docs/design/archetypes/generated-section/)
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

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Document Index

<?catalog
glob: "data/*.md"
row: "[{{.filename}}](good/{{.filename}})"
?>
[data/alpha.md](good/data/alpha.md)
[data/beta.md](good/data/beta.md)
<?/catalog?>
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Document Index

<?catalog
glob: "data/*.md"
?>
- [alpha.md](bad/data/alpha.md)
<?/catalog?>
```

<?/include?>

## Diagnostics

| Condition      | Message                      |
|----------------|------------------------------|
| Missing `glob` | `...missing required "glob"` |
| Empty `glob`   | `...has empty "glob"`        |
| Absolute glob  | `...has absolute glob path`  |

| Condition      | Message                   |
|----------------|---------------------------|
| Glob with `..` | `...".." path traversal`  |
| Invalid glob   | `...invalid glob pattern` |
| Empty sort     | `...empty "sort" value`   |
| Invalid sort   | `...invalid sort value`   |

All messages above are prefixed with
`generated section directive`. Column is always 1.

See the
[archetype documentation](../../../docs/design/archetypes/generated-section/)
for shared diagnostics (content mismatch, unclosed markers,
nested markers, YAML errors, template errors).

## Edge Cases

| Scenario             | Behavior          |
|----------------------|-------------------|
| No front matter      | Others -> empty   |
| Invalid front matter | Treated as absent |
| Missing field        | Empty string      |
| Unreadable file      | Skipped           |

| Scenario                 | Behavior                  |
|--------------------------|---------------------------|
| Glob matches directory   | Skipped                   |
| Glob matches linted file | Included                  |
| Binary file              | Included; no front matter |
| Symlinks                 | Followed                  |

| Scenario           | Behavior                       |
|--------------------|--------------------------------|
| Dotfiles           | Matched by `*`/`**`            |
| Absolute/`..` glob | Diagnostic                     |
| Brace expansion    | Supported                      |
| Multi-glob list    | Union of matches, deduplicated |
| Empty glob/sort    | Diagnostic                     |

| Scenario             | Behavior         |
|----------------------|------------------|
| `sort: "-"`          | Diagnostic       |
| Sort with whitespace | Diagnostic       |
| No files matched     | `empty` fallback |
| Files + `empty`      | `empty` ignored  |

See the
[archetype documentation](../../../docs/design/archetypes/generated-section/)
for shared edge cases (markers in code blocks, multiple marker
pairs, line endings, template errors).
