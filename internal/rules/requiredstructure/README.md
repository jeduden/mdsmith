---
id: MDS020
name: required-structure
description: Document must match the heading structure defined by its template.
---
# MDS020: required-structure

Document must match the heading structure defined by its
template.

- **ID**: MDS020
- **Name**: `required-structure`
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

## Config

Enable with a template for rule READMEs:

```yaml
overrides:
  - files: ["rules/*/README.md"]
    rules:
      required-structure:
        template: rules/proto.md
```

Disable:

```yaml
rules:
  required-structure: false
```

## Examples

### Good

```markdown
# MDS001: line-length

Line exceeds maximum length.

## Settings

## Examples

### Good

### Bad
```

### Bad

```markdown
# MDS001: line-length

Line exceeds maximum length.

## Examples
```

## Diagnostics

| Condition       | Message                                                                       |
|-----------------|-------------------------------------------------------------------------------|
| section missing | missing required section "## Settings"                                        |
| wrong level     | heading level mismatch: expected h2, got h3                                   |
| extra section   | unexpected section "## Extra"                                                 |
| heading sync    | heading does not match frontmatter: expected "MDS001" (from id), got "MDS002" |
| body sync       | body does not match frontmatter field "description"                           |
