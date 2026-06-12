---
id: MDS019
name: catalog
status: ready
description: Catalog content must reflect selected front matter fields from files matching its glob.
category: directive
nature: directive
maintainability:
  signal: a list of links to sibling files in the same directory
  fix: adopt a `<?catalog?>` directive so the list stays in sync
  for-diagnostic: false
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS019: catalog

Catalog content must reflect selected front matter fields
from files matching its glob.

## Directive: `catalog`

Lists files matching a glob pattern. Uses *template mode*
when the YAML body has a `row` key, and *minimal mode*
otherwise.

### Parameters

| Parameter  | Required | Default | Description                         |
| ---------- | -------- | ------- | ----------------------------------- |
| `glob`     | yes      | --      | Relative file glob                  |
| `sort`     | no       | `path`  | Sort key                            |
| `where`    | no       | --      | CUE filter on parsed front matter   |
| `row`      | no       | --      | Placeholder-style per-file template |
| `row-expr` | no       | --      | CUE expression per-file template    |
| `header`   | no       | --      | Literal text emitted above the rows |
| `footer`   | no       | --      | Literal text emitted below the rows |
| `empty`    | no       | --      | Literal text when no files match    |
| `columns`  | no       | --      | Column width/wrapping (placeholder) |

`row` and `row-expr` are mutually exclusive; setting both
on the same directive emits an MDS019 diagnostic. Either
form (or neither, for minimal mode) is allowed.

The `glob` accepts a single string or a YAML list of
strings. It supports `*`, `?`, `[...]`, `**`, and `{a,b}`
brace expansion.

Absolute paths are rejected. `..` segments are allowed as
long as the resolved pattern stays inside the project root;
a pattern whose resolution leaves the root is rejected with
`generated section directive glob escapes project root`. A
`..` pattern without a configured project root is rejected
with `generated section directive glob contains ".." but
project root is not configured`. This mirrors how
[`<?include?>`](../MDS021-include/README.md) resolves its
`file` parameter.

A `..` segment inside a `{a,b}` brace alternative (e.g.
`{..,sibling}/*.md`) is rejected at validation time —
brace alternatives cannot be normalized before glob
expansion, so a pattern that mixes `..` with braces is
written as separate top-level patterns instead.

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
[generated-section concept](../../../docs/background/concepts/generated-section.md)
for details.

### Template fields

The `row` section uses `{fieldname}` placeholder syntax.

- `{filename}` -- relative path from the marker file's
  directory (no leading `./`).
- Other names (e.g., `{title}`) -- looked up in the
  matched file's YAML front matter.
- Missing field -> empty string.
- Case-mismatched field (e.g., `{Title}` when
  front matter has `title`) -> "did you mean?" hint.
- Non-string scalar (number, bool) -> formatted to
  string. Composite values (maps, slices) -> empty
  string.
- Literal `{` is written as `{{`, literal `}` as `}}`.

### Column constraints

The `columns` parameter sets per-column width limits. Each
key is a template field name. Options:

| Option      | Type   | Default    | Description          |
| ----------- | ------ | ---------- | -------------------- |
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

### Filtering with `where`

The `where` parameter accepts a CUE struct-literal body
matched against each file's parsed front matter. The
grammar is the same one
[`mdsmith list query`](../../../docs/reference/cli/query.md)
uses, so a working `mdsmith list query` expression drops
in unchanged. Files whose front matter fails the
expression are excluded before sort and render.

```yaml
where: 'nature: "directive"'
```

Failure modes:

- Invalid CUE expression -> directive emits a diagnostic
  on its opening line.
- Front matter is missing the referenced field -> file
  is excluded.
- Field exists but its type does not match -> file is
  excluded.

### CUE-expression rows via `row-expr`

`row` interpolates `{field}` placeholders against
scalar fields only. `row-expr` is the list-typed
alternative. It compiles a CUE expression once and
evaluates it per matched file. Every
identifier-safe key binds at top-level scope.

```yaml
row-expr: |
  strings.Join(
    [for m in markdownlint {
      "\(m.id) \([if m.default {"✅"}, if !m.default {"⚪"}][0]) \(m.name)"
    }],
    ", "
  )
```

The `strings` package is preimported. CUE has no
infix ternary; the `[if cond {a}, if !cond {b}][0]`
idiom selects between two strings on a boolean.
`row` and `row-expr` are mutually exclusive;
`columns:` applies to `row` only.

### Minimal mode

Without `row`, `row-expr`, `header`, or `footer`, the
directive outputs a bullet list:
`- [<basename>](<relative-path>)`. Front matter is only
read when the sort key needs it.

### Rendering logic

1. Files matched: `header` + rows + `footer` (`empty` ignored)
2. No files, `empty` defined: `empty` text
3. No files, no `empty`: zero lines between markers

See the
[generated-section concept](../../../docs/background/concepts/generated-section.md)
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

The lint-test fixture lives at
[good/default.md](good/default.md):

```markdown
# Document Index

<?catalog
glob: "data/*.md"
row: "[{filename}]({filename})"
?>
[data/alpha.md](data/alpha.md)
[data/beta.md](data/beta.md)
<?/catalog?>
```

### Bad

The lint-test fixture lives at
[bad/default.md](bad/default.md). The body shows
only one entry while the glob matches two, so the
generated section is out of date:

```markdown
# Document Index

<?catalog
glob: "data/*.md"
row: "[{filename}]({filename})"
?>
[data/alpha.md](data/alpha.md)
<?/catalog?>
```

## Diagnostics

| Condition      | Message                      |
| -------------- | ---------------------------- |
| Missing `glob` | `...missing required "glob"` |
| Empty `glob`   | `...has empty "glob"`        |
| Absolute glob  | `...has absolute glob path`  |

| Condition          | Message                                                                  |
| ------------------ | ------------------------------------------------------------------------ |
| Glob escapes root  | `...glob escapes project root`                                           |
| `..` without root  | `...glob contains ".." but project root is not configured`               |
| File outside root  | `...catalog file is outside project root; ".." globs cannot be resolved` |
| `..` inside braces | `...has ".." inside brace expansion; rewrite as separate patterns`       |
| Invalid glob       | `...invalid glob pattern`                                                |
| Empty sort         | `...empty "sort" value`                                                  |
| Invalid sort       | `...invalid sort value`                                                  |
| Invalid `where`    | `...has invalid "where" expression`                                      |
| Invalid `row-expr` | `...has invalid "row-expr" expression` (or empty)                        |
| `row` + `row-expr` | `...sets both "row" and "row-expr"; choose one`                          |

All messages above are prefixed with
`generated section directive`. Column is always 1.

See the
[generated-section concept](../../../docs/background/concepts/generated-section.md)
for shared diagnostics (content mismatch, unclosed markers,
nested markers, YAML errors, template errors).

## Edge Cases

| Scenario              | Behavior             |
| --------------------- | -------------------- |
| No front matter       | Others -> empty      |
| Invalid front matter  | Treated as absent    |
| Missing field         | Empty string         |
| Case-mismatched field | "did you mean?" hint |
| Unreadable file       | Skipped              |

| Scenario                 | Behavior                  |
| ------------------------ | ------------------------- |
| Glob matches directory   | Skipped                   |
| Glob matches linted file | Included                  |
| Binary file              | Included; no front matter |
| Symlinks                 | Followed                  |

| Scenario          | Behavior                       |
| ----------------- | ------------------------------ |
| Dotfiles          | Matched by `*`/`**`            |
| Absolute glob     | Diagnostic                     |
| `..` inside root  | Resolved against project root  |
| `..` escapes root | Diagnostic                     |
| Brace expansion   | Supported                      |
| Multi-glob list   | Union of matches, deduplicated |
| Empty glob/sort   | Diagnostic                     |

| Scenario             | Behavior         |
| -------------------- | ---------------- |
| `sort: "-"`          | Diagnostic       |
| Sort with whitespace | Diagnostic       |
| No files matched     | `empty` fallback |
| Files + `empty`      | `empty` ignored  |

See the
[generated-section concept](../../../docs/background/concepts/generated-section.md)
for shared edge cases (markers in code blocks, multiple marker
pairs, line endings, template errors).

## Pattern

The bad pattern is a hand-maintained list of
sibling links; the good pattern is the same
content rewritten as a `<?catalog?>` directive.
Canonical files: [pattern/bad/](pattern/bad/),
[pattern/good/](pattern/good/). Snippets below
mirror them; the markdown-audit skill reads the
folders directly.

### Without the directive

```markdown
# Project Index

- [Alpha](data/alpha.md)
- [Beta](data/beta.md)
```

### With the directive

```markdown
# Project Index

<?catalog
glob: "data/*.md"
row: "- [{title}]({filename})"
?>
- [Alpha](data/alpha.md)
- [Beta](data/beta.md)
<?/catalog?>
```

## Meta-Information

- **ID**: MDS019
- **Name**: `catalog`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: directive
- **Concept**:
  [generated-section](../../../docs/background/concepts/generated-section.md)
- **Guide**:
  [directive guide](../../../docs/guides/directives/generating-content.md)
