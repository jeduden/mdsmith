---
id: MDS020
name: required-structure
status: ready
description: Document structure and front matter must match its template.
---
# MDS020: required-structure

Document structure and front matter must match its
template.

- **ID**: MDS020
- **Name**: `required-structure`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting  | Type   | Default | Description           |
|----------|--------|---------|-----------------------|
| `template` | string | `""`      | Path to template file |

When `template` is empty the rule is a no-op. Use
overrides to apply templates to specific file groups.

Template front matter may embed a CUE schema that validates
document front matter:

```yaml
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
```

Template body controls section strictness:

- By default, extra sections are rejected.
- Add a heading with text `...` (for example `## ...`) to
  allow extra headings in that position until the next
  required heading anchor.

## Config

Enable with a template for rule READMEs:

```yaml
overrides:
  - files: ["internal/rules/*/README.md"]
    rules:
      required-structure:
        template: internal/rules/proto.md
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
| front matter schema | front matter does not satisfy template CUE schema: ...                        |
