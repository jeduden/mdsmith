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

### Project a list-typed field into a row

`{field}` placeholders reach scalar front-matter
values only. To project a list — multiple authors
per file, multiple peer-linter mappings on a rule
README, multiple tags — switch the directive from
`row:` to `row-expr:`. The value is a CUE
expression. Every identifier-safe key in the
matched file's front matter binds at top-level
scope. The expression must return a string.

```markdown
<?catalog
glob: "internal/rules/MDS*/README.md"
where: 'category: "heading"'
sort: id
row-expr: |
  "| \(id) | " +
  strings.Join(
    [for m in markdownlint {
      "\(m.id) \([if m.default {"✅"}, if !m.default {"⚪"}][0]) \(m.name)"
    }],
    ", "
  ) + " |"
?>
<?/catalog?>
```

The `strings` standard-library package is
preimported. CUE has no infix ternary; pick
between two values with the
`[if cond {a}, if !cond {b}][0]` list-comprehension
idiom. `row` and `row-expr` are mutually exclusive
on one directive. `columns:` width constraints
apply to `row` only.

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

### Filtering with `where`

The `where` parameter narrows the matched set by a CUE
expression
evaluated against each file's parsed front matter. The
expression grammar is the same one
[`mdsmith list query`](../../reference/cli/query.md)
accepts, so a CLI query expression drops in unchanged:

```markdown
<?catalog
glob: "internal/rules/MDS*/README.md"
where: 'nature: "directive"'
row: "- [{name}]({filename})"
?>
<?/catalog?>
```

The filter runs after globbing and front matter
parsing, but before sort and render. Failure modes:

- An invalid CUE expression triggers an MDS019
  diagnostic on the directive's opening line.
- A file whose front matter is missing the referenced
  field is excluded (no diagnostic).
- A field whose value does not satisfy the constraint
  (wrong type or wrong value) is excluded.

### Sorting

Format: `[-][numeric:]KEY`. A `-` prefix means
descending. Built-in keys: `path`, `filename`. Any
other key is looked up in front matter. Missing
values sort as empty string.

The optional `numeric:` prefix opts a field into
integer comparison. Use it for ID-shaped fields
where mixed 2-digit and 3-digit values would
otherwise collate lexicographically (`100` before
`52`):

```markdown
<?catalog
glob: "plan/*.md"
sort: numeric:id
row: "| {id} | [{title}]({filename}) |"
?>
```

If any matched file's value fails to parse as an
integer, the directive falls back to string
compare for all entries — no error is raised, so a
field that is sometimes numeric stays usable.
`-numeric:id` reverses the order.

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

### Heading-offset adjustment

To shift every included heading by a fixed amount, use
`heading-offset` with a signed integer from -6 to 6:

```markdown
<?include
file: features.md
heading-offset: "1"
?>
## Was An H1 In The Source
<?/include?>
```

A positive value demotes headings; a negative value
promotes them. Unlike `heading-level: "absolute"`, the
shift does not depend on a preceding heading, so it also
works at the document root — handy when a file's visual
title is a logo or image rather than an H1.
`heading-offset` cannot combine with `heading-level` or
`extract:` — those parameters are mutually exclusive with
it.

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

### Include a typed value

When the target file is kind-typed, `extract:` walks
the same JSON projection
[`mdsmith extract`](../../reference/cli/extract.md)
produces and splices a single leaf. The value is a
dotted path: `frontmatter.title` reaches the title
field; `tagline.text` reaches the paragraph under
the `## Tagline` heading; `headline.code` reaches a
fenced code block.

A paragraph section. The target's
`## Tagline` heading projects to `{"tagline":
{"text": "..."}}`, so the path `tagline.text` lands
on the prose:

```markdown
<?include
file: docs/brand/messaging.md
extract: tagline.text
?>
Markdown, fast.
<?/include?>
```

A fenced code block. The `## Headline` section
projects to `{"headline": {"code": "..."}}`. Use
`headline.code` to splice the code body verbatim:

```markdown
<?include
file: docs/brand/messaging.md
extract: headline.code
?>
Mark*down*, smithed.
<?/include?>
```

A frontmatter scalar. Every kind-typed file's
projection carries a top-level `frontmatter` object
beside the body sections. The path is
`frontmatter.<key>`:

```markdown
<?include
file: docs/brand/messaging.md
extract: frontmatter.title
?>
mdsmith product messaging
<?/include?>
```

Single-content-key shortcut. A leaf object that
carries exactly one of `text`, `code`, `items`, or
`rows` resolves to the inner value, so
`extract: tagline` and `extract: tagline.text`
splice the same content. Multi-key wrappers (the
JSON projection of a section with several content
entries) are ambiguous and surface as a lint error
with the available keys listed.

Restrictions. `extract:` is scalar-only for now —
combining it with `strip-frontmatter:` or
`heading-level:` is rejected. A target file with no
resolved kind is rejected too: without a schema there
is no projection to walk. A target whose body fails
`required-structure` surfaces the same diagnostic
`mdsmith check` would raise on the target, anchored
to the include directive's call site.

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
