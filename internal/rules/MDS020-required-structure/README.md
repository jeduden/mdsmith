---
id: MDS020
name: required-structure
status: ready
description: Document structure and front matter must match its schema.
---
# MDS020: required-structure

Document structure and front matter must match its
schema.

- **ID**: MDS020
- **Name**: `required-structure`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**: [source](./)
- **Category**: meta

## Settings

| Setting  | Type   | Default | Description         |
|----------|--------|---------|---------------------|
| `schema` | string | `""`    | Path to schema file |

When `schema` is empty the rule skips structure and
front matter validation, but still warns on misplaced
`<?require?>` directives. Use overrides to apply
schemas to specific file groups.

Schema front matter may embed a CUE schema that
validates document front matter:

```yaml
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
```

### Require directive

Use `<?require?>` in the schema body to declare
constraints on files validated against this schema:

| Field      | Type   | Description                           |
|------------|--------|---------------------------------------|
| `filename` | string | Glob the document basename must match |

```markdown
<?require
filename: "[0-9]*_*.md"
?>
```

### Schema composition

Schema files can use `<?include?>` to share
structure across schemas. Included fragment
headings are spliced into the heading list at
the include position. Fragment front matter is
ignored. `<?require?>` from fragments is merged.

```markdown
# ?

## Goal

<?include
file: common/acceptance-criteria.md
?>
```

Cycle detection prevents circular includes.
Max include depth is 10.

### Optional fields

Append `?` to a schema front matter key to make it
optional. The field may be absent in the document,
but if present it must satisfy the type constraint:

```yaml
name: 'string & != ""'
"description?": string
```

Schema body controls section strictness:

- By default, extra sections are rejected.
- Add a heading with text `...` (for example `## ...`) to
  allow extra headings in that position until the next
  required heading anchor.

## Config

Enable with a schema for rule READMEs:

```yaml
overrides:
  - files: ["internal/rules/*/README.md"]
    rules:
      required-structure:
        schema: internal/rules/proto.md
```

Disable:

```yaml
rules:
  required-structure: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# My Plan

## Goal

Describe the goal here.

## Tasks

List tasks here.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# My Plan

## Goal

Describe the goal here.
```

<?/include?>

## Diagnostics

| Condition           | Message                                                                       |
|---------------------|-------------------------------------------------------------------------------|
| section missing     | missing required section "## Settings"                                        |
| wrong level         | heading level mismatch: expected h2, got h3                                   |
| extra section       | unexpected section "## Extra"                                                 |
| heading sync        | heading does not match frontmatter: expected "MDS001" (from id), got "MDS002" |
| body sync           | body does not match frontmatter field "description"                           |
| front matter schema | front matter does not satisfy schema CUE constraints: ...                     |
| filename mismatch   | filename "foo.md" does not match required pattern "[0-9]*_*.md"               |
| misplaced require   | <?require?> is only recognized in schema files; this directive has no effect  |
| schema include loop | cyclic include: a.md -> b.md -> a.md                                          |
