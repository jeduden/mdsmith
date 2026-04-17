---
title: Generating Content with Directives
summary: >-
  How to use catalog and include directives to
  generate and embed content in Markdown files.
---
# Generating Content with Directives

mdsmith can generate content inside your Markdown
files. Two directives handle this: `<?catalog?>` for
building file indexes and `<?include?>` for embedding
content from other files. Both regenerate their body
on `mdsmith fix` and flag stale content on
`mdsmith check`.

## Building a file index

Use `<?catalog?>` when you need a list of files with
metadata — for example, a table of plans, a docs
index, or a rule directory.

### Minimal catalog

The simplest form lists all matching files as a
bullet list:

```markdown
<?catalog
glob: "docs/**/*.md"
?>
- [cli.md](design/cli.md)
- [index.md](development/index.md)
<?/catalog?>
```

Without `row`, `header`, or `footer`, the directive
outputs `- [<basename>](<path>)` per file, sorted by
path.

### Custom template

Add `row` (and optionally `header`, `footer`) to
control the output format. Use `{field}` placeholders
to pull front matter values from matched files:

```markdown
<?catalog
glob: "plan/*.md"
sort: id
header: |
  | ID | Title |
  |----|-------|
row: "| {id} | [{title}]({filename}) |"
?>
| 74 | [Directive guide](plan/74_directive-guide.md) |
| 75 | [Single-brace placeholders](plan/75_single-brace-placeholders.md) |
<?/catalog?>
```

### Multiple glob patterns

`glob` accepts a YAML list to collect files from
several directories:

```markdown
<?catalog
glob:
  - "docs/**/*.md"
  - "plan/*.md"
sort: path
row: "- [{title}]({filename})"
?>
```

### Excluding files

Prefix a glob pattern with `!` to exclude matching
files. Exclusion patterns use the same glob syntax as
include patterns:

```markdown
<?catalog
glob:
  - "**/*.md"
  - "!drafts/**"
  - "!internal/notes.md"
row: "- [{title}]({filename})"
?>
```

At least one non-negated (include) pattern is
required. Excludes and includes are collected into
separate lists — a file matching any exclude pattern
is filtered out regardless of where the pattern
appears in the list.

### Gitignore filtering

By default, files matched by `.gitignore` rules are
excluded from catalog results. To include gitignored
files, set `gitignore: "false"`:

```markdown
<?catalog
glob: "**/*.md"
gitignore: "false"
?>
```

### Sorting

Format: `[-]KEY`. A `-` prefix means descending.
Built-in keys: `path`, `filename`. Any other key is
looked up in front matter. Missing values sort as
empty string.

### What happens when no files match

If `empty` is defined, its text is used. Otherwise
zero lines appear between the markers.

For full parameter reference, see
[MDS019 catalog](../../../internal/rules/MDS019-catalog/README.md).

## Embedding file content

Use `<?include?>` when you want to embed the content
of another file — for example, sharing a development
guide across README and docs, or including a config
file as a code block.

### Basic include

```markdown
<?include
file: DEVELOPMENT.md
strip-frontmatter: "true"
?>
Build and test reference for contributors.
<?/include?>
```

By default, YAML front matter is stripped. Set
`strip-frontmatter: "false"` to keep it.

### Code fence wrapping

Include a non-Markdown file wrapped in a fenced code
block:

````markdown
<?include
file: config.yml
wrap: yaml
?>
```yaml
key: value
```
<?/include?>
````

### Heading-level adjustment

When including under an existing heading, use
`heading-level: "absolute"` to shift included
headings so they nest correctly:

```markdown
## Project

<?include
file: DEVELOPMENT.md
heading-level: "absolute"
?>
### Build
Steps here.
<?/include?>
```

Without this parameter, included headings keep their
original level, which may break heading hierarchy.

### Link rewriting

Relative links in included content are automatically
rewritten to resolve from the including file's
directory. Absolute URLs and protocol links are not
modified.

### Cycle detection

Include chains are tracked. Circular includes and
chains deeper than 10 levels are rejected:

```text
cyclic include: a.md -> b.md -> a.md
```

For full parameter reference, see
[MDS021 include](../../../internal/rules/MDS021-include/README.md).

## Placement rules

Both directives are only recognized at **document
root** (parent must be the Document node). Maximum
indent is 3 spaces.

Directives are **not** recognized inside list items,
blockquotes, tables, fenced code blocks, or HTML
blocks.

**4-space indent footgun**: 4 or more leading spaces
turns the line into a code block. The directive is
silently ignored with **no error**. Always use 0-3
spaces.

## Nesting

Same-type nesting is supported. When an included file
contains its own generated sections (include, catalog,
etc.), the inner markers are treated as literal content
of the outer directive. `FindMarkerPairs` pairs only the
outermost markers of each directive type; inner markers
are skipped. Cross-type directives between markers may
appear but are overwritten by the outer generator on
`fix`.

## Placeholder syntax

`{field}` is the only placeholder syntax. It works in
`row` templates and schema headings.

- `{filename}` — relative path from the marker file.
- `{title}`, `{summary}` — looked up in front matter.
- Missing field — empty string.
- Case mismatch — "did you mean?" hint.
- Non-string scalar — formatted to string. Composite
  values (maps, slices) — empty string.
- Literal `{` — write `{{`. Literal `}` — write `}}`.

CUE paths provide nested access for structured front
matter values.
