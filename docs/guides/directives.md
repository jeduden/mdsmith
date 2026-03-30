---
title: Directive Guide
summary: >-
  Central guide for all directives and rules with
  examples, fixability table, placement rules, and
  nesting behavior.
---
# Directive Guide

This guide documents every directive and rule in
mdsmith. It covers placement rules, parameters,
nesting behavior, placeholder syntax, schema
composition, and fixability.

## Quick Reference

| Directive                 | Purpose                                      | Closing tag | Fixable | Key parameters                            |
|---------------------------|----------------------------------------------|-------------|---------|-------------------------------------------|
| `<?catalog?>`             | List files matching a glob with front matter | yes         | yes     | `glob`, `sort`, `row`, `header`, `footer` |
| `<?include?>`             | Embed content from another file              | yes         | yes     | `file`, `strip-frontmatter`, `wrap`       |
| `<?require?>`             | Declare constraints in schema files          | no          | no      | `filename`                                |
| `<?allow-empty-section?>` | Suppress empty-section diagnostic            | no          | no      | none                                      |

Two kinds of directives:

- **With closing tag** (`<?name?>...<?/name?>`):
  `fix` regenerates the body between markers.
  `check` reports when the body is out of date.
- **Without closing tag** (`<?name?>`): `check`
  validates a condition. Nothing to regenerate.

## Placement Rules

Directives are HTML processing instructions (PIs).
They are only recognized at document root, where the
parent node is the Document node in the AST. The
maximum indent is 3 spaces.

Directives are **not** recognized inside:

- list items
- blockquotes
- tables
- fenced code blocks
- HTML blocks

**4-space indent footgun**: At document root, 4 or
more leading spaces turns the line into an indented
code block. The directive is silently ignored with
**no error emitted**. Always use 0-3 spaces of
indent.

```markdown
<!-- Good: recognized as a directive -->
<?catalog
glob: "docs/*.md"
?>
<?/catalog?>

<!-- Bad: 4 spaces = code block, silently ignored -->
    <?catalog
    glob: "docs/*.md"
    ?>
    <?/catalog?>
```

## Directives

### `<?catalog?>`

Lists files matching a glob pattern and renders
their front matter fields.

**Rule**: [MDS019](../../internal/rules/MDS019-catalog/README.md) |
**Fixable**: yes |
**Archetype**: [generated-section](../design/archetypes/generated-section/)

#### Catalog parameters

| Parameter | Required | Default | Description                |
|-----------|----------|---------|----------------------------|
| `glob`    | yes      | --      | File glob pattern(s)       |
| `sort`    | no       | `path`  | Sort key: `[-]KEY`         |
| `row`     | no       | --      | Per-entry template         |
| `header`  | no       | --      | Static text before entries |
| `footer`  | no       | --      | Static text after entries  |
| `empty`   | no       | --      | Fallback when no matches   |
| `columns` | no       | --      | Column width constraints   |

`glob` accepts a single string or a YAML list. It
supports `*`, `?`, `[...]`, `**`, and `{a,b}` brace
expansion. Absolute paths and `..` traversal are
rejected.

`sort` format: `[-]KEY`. A `-` prefix means
descending. Built-in keys: `path`, `filename`. Other
keys are looked up in front matter.

Without `row`, `header`, or `footer`, the directive
outputs a bullet list: `- [<basename>](<path>)`.

#### Catalog example

```markdown
<?catalog
glob: "docs/**/*.md"
sort: path
header: ""
row: "- [{summary}]({filename})"
?>
- [Build commands](development/index.md)
- [Metrics guide](guides/metrics-tradeoffs.md)
<?/catalog?>
```

#### Catalog diagnostics

- `generated section is out of date` when body
  does not match expected output.
- `missing required "glob"` when glob is absent.
- `has absolute glob path` when glob starts with `/`.
- `fix` regenerates content between `<?catalog?>`
  and `<?/catalog?>` from matched files.

### `<?include?>`

Embeds content from another file.

**Rule**: [MDS021](../../internal/rules/MDS021-include/README.md) |
**Fixable**: yes |
**Archetype**: [generated-section](../design/archetypes/generated-section/)

#### Include parameters

| Parameter           | Required | Default  | Description                           |
|---------------------|----------|----------|---------------------------------------|
| `file`              | yes      | --       | Relative path to the file to include  |
| `strip-frontmatter` | no       | `"true"` | Remove YAML front matter from content |
| `wrap`              | no       | --       | Wrap in fenced code block (language)  |
| `heading-level`     | no       | --       | `"absolute"`: shift headings to nest  |

Relative links in included content are rewritten to
resolve from the including file's directory. Absolute
URLs, anchor-only links, and protocol links are not
modified.

Cycle detection prevents circular includes. Max
include depth is 10.

#### Include example

```markdown
<?include
file: DEVELOPMENT.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
### Build
Steps here.
<?/include?>
```

#### Include diagnostics

- `generated section is out of date` when body
  does not match expected output.
- `include file "x.md" not found` when file is
  missing.
- `cyclic include: a.md -> b.md -> a.md` on cycles.
- `fix` regenerates content between `<?include?>`
  and `<?/include?>` from the referenced file.

### `<?require?>`

Declares constraints on files validated against a
schema. Only recognized in schema files.

**Rule**: [MDS020](../../internal/rules/MDS020-required-structure/README.md) |
**Fixable**: no

#### Require parameters

| Parameter  | Required | Default | Description                           |
|------------|----------|---------|---------------------------------------|
| `filename` | string   | --      | Glob the document basename must match |

#### Require example

```markdown
<?require
filename: "[0-9]*_*.md"
?>
```

#### Require diagnostics

- `filename "foo.md" does not match required
  pattern "[0-9]*_*.md"` when a document's basename
  does not match.
- `<?require?> is only recognized in schema files;
  this directive has no effect` when used outside a
  schema file.
- No closing tag. Nothing to fix.

### `<?allow-empty-section?>`

Suppresses the empty-section diagnostic
([MDS030](../../internal/rules/MDS030-empty-section-body/README.md))
for the section it appears in.

**Rule**: [MDS030](../../internal/rules/MDS030-empty-section-body/README.md) |
**Fixable**: no

No parameters. No closing tag.

#### Allow-empty-section example

```markdown
## Compatibility

<?allow-empty-section?>

## Notes

This section has real content.
```

#### Allow-empty-section diagnostics

With the marker present, MDS030 skips the section.
Without it, MDS030 reports: `section "## Heading"
has no meaningful body content`. Nothing to fix.

## Nesting

Same-type nesting is prohibited. Placing a
`<?catalog?>` inside another `<?catalog?>` emits:
`nested generated section markers are not allowed`.

Cross-type directives between markers (for example,
an `<?include?>` inside a `<?catalog?>`) may appear
but are **overwritten** by the outer generator on
`fix`. Generated output is not re-parsed for further
directives.

## Placeholder Syntax

`{field}` is the only placeholder syntax. It works
in both catalog `row` templates and schema headings.

- `{filename}` -- relative path from the marker
  file's directory.
- `{title}`, `{summary}`, etc. -- looked up in the
  matched file's YAML front matter.
- Missing field -- empty string.
- Case-mismatched field (e.g. `{Title}` when front
  matter has `title`) -- "did you mean?" hint.
- Non-string value -- Go default string format.
- Literal `{` -- write `{{`. Literal `}` -- write
  `}}`.

CUE paths provide nested access for structured front
matter values.

## Schema vs Normal File

Schemas enforce **headings and front matter only**,
not directives. A `<?catalog?>` in a schema does not
require documents to also contain one.

Key differences:

| Behavior                         | Schema file        | Normal file          |
|----------------------------------|--------------------|----------------------|
| `<?require?>` recognized         | yes                | no (warning emitted) |
| `<?allow-empty-section?>` effect | local to that file | local to that file   |
| `<?include?>` in schema          | splices headings   | embeds content       |
| Directives enforced on docs      | no                 | n/a                  |

`<?allow-empty-section?>` does **not** propagate
from a schema to documents that use it. Each file
must add its own marker.

`<?require?>` is **schema-only**. Using it in a
normal file emits: `<?require?> is only recognized in
schema files; this directive has no effect`.

## Schema Composition

Schema files can use `<?include?>` to share structure
across schemas. Included fragment headings are spliced
into the heading list at the include position.
Fragment front matter is ignored. `<?require?>`
constraints from fragments are merged.

```markdown
# ?

## Goal

<?include
file: common/acceptance-criteria.md
?>
```

Cycle detection prevents circular includes. Max
include depth is 10. The cycle message shows the full
chain: `cyclic include: a.md -> b.md -> a.md`.

## Renamed Parameters

These parameters were renamed for clarity:

| Old name                    | New name                 | Rule   |
|-----------------------------|--------------------------|--------|
| `ratio`                     | `words-per-token`        | MDS028 |
| `max-words`                 | `max-words-per-sentence` | MDS024 |
| `max-column-width-variance` | `max-column-width-ratio` | MDS026 |

## Fixability Summary

All 33 rules and whether `fix` can auto-correct
violations.

### Fixable rules (MDS001-MDS016)

| Rule   | Name                            | Fixable |
|--------|---------------------------------|---------|
| MDS001 | `line-length`                   | no      |
| MDS002 | `heading-style`                 | yes     |
| MDS003 | `heading-increment`             | no      |
| MDS004 | `first-line-heading`            | no      |
| MDS005 | `no-duplicate-headings`         | no      |
| MDS006 | `no-trailing-spaces`            | yes     |
| MDS007 | `no-hard-tabs`                  | yes     |
| MDS008 | `no-multiple-blanks`            | yes     |
| MDS009 | `single-trailing-newline`       | yes     |
| MDS010 | `fenced-code-style`             | yes     |
| MDS011 | `fenced-code-language`          | no      |
| MDS012 | `no-bare-urls`                  | yes     |
| MDS013 | `blank-line-around-headings`    | yes     |
| MDS014 | `blank-line-around-lists`       | yes     |
| MDS015 | `blank-line-around-fenced-code` | yes     |
| MDS016 | `list-indent`                   | yes     |

### Fixable rules (MDS017-MDS033)

| Rule   | Name                                 | Fixable |
|--------|--------------------------------------|---------|
| MDS017 | `no-trailing-punctuation-in-heading` | no      |
| MDS018 | `no-emphasis-as-heading`             | no      |
| MDS019 | `catalog`                            | yes     |
| MDS020 | `required-structure`                 | no      |
| MDS021 | `include`                            | yes     |
| MDS022 | `max-file-length`                    | no      |
| MDS023 | `paragraph-readability`              | no      |
| MDS024 | `paragraph-structure`                | no      |
| MDS025 | `table-format`                       | yes     |
| MDS026 | `table-readability`                  | no      |
| MDS027 | `cross-file-reference-integrity`     | no      |
| MDS028 | `token-budget`                       | no      |
| MDS029 | `conciseness-scoring`                | no      |
| MDS030 | `empty-section-body`                 | no      |
| MDS031 | `unclosed-code-block`                | no      |
| MDS032 | `no-empty-alt-text`                  | no      |
| MDS033 | `directory-structure`                | no      |

## Coming from Hugo

If you are familiar with Hugo templates, here are
the key differences in mdsmith:

- **Placeholder syntax**: Use `{title}` not
  `{{ .Title }}`. Field names are case-sensitive and
  match front matter keys exactly. There are no Go
  templates in user-facing syntax. If you use the
  wrong case (e.g. `{Title}` when front matter has
  `title`), mdsmith emits a "did you mean?" hint.

- **Schema is not a rendering template**: The
  `schema` config key defines a validation contract.
  It checks that documents have the required headings
  and front matter. It does not render output like a
  Hugo layout.

- **Generated content is committed**: Content between
  directive markers is committed to git. It is not
  gitignored. Run `mdsmith fix` to regenerate, then
  commit the result.

- **Schema composition**: Schema files compose via
  `<?include?>`, which splices headings from fragment
  files. This is not Hugo's `partial` or `block`
  system.

- **No nesting in normal files**: Directives are
  flat. There is no equivalent of Hugo's nested
  `block`/`define`.

- **CUE paths, not Go template syntax**: `{field}`
  uses CUE path semantics for nested front matter
  access, not Go template dot notation.

- **Directive params are YAML strings**: All
  parameter values in the YAML body of a directive
  must be strings. Non-string values (numbers,
  booleans, null) produce a diagnostic.
