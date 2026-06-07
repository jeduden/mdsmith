---
id: MDS003
name: heading-increment
status: ready
description: Heading levels should increment by one. No jumping from `#` to `###`.
category: heading
nature: structure
maintainability: null
markdownlint:
  - id: MD001
    name: heading-increment
    partial: false
    default: true
rumdl:
  - id: MD001
    name: heading-increment
    partial: false
    default: true
mado:
  - id: MD001
    name: heading-increment
    partial: false
    default: true
panache:
  - id: heading-hierarchy
    name: heading-hierarchy
    partial: false
    default: true
obsidian-linter:
  - id: header-increment
    name: header-increment
    partial: false
    default: false
---
# MDS003: heading-increment

Heading levels should increment by one. No jumping from `#` to `###`.

## Settings

| Setting        | Type | Default | Description                                                                                                                |
| -------------- | ---- | ------- | -------------------------------------------------------------------------------------------------------------------------- |
| `placeholders` | list | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Useful tokens: `heading-question`, `placeholder-section`, `var-token`.

## Config

Enable:

```yaml
rules:
  heading-increment: true
```

Disable:

```yaml
rules:
  heading-increment: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

### Subsection
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

## Section

Body text.
```

<?/include?>

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)

## Meta-Information

- **ID**: MDS003
- **Name**: `heading-increment`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading
- **markdownlint**: [MD001][mdl-md001] (heading-increment)
- **rumdl**: [MD001][rumdl-md001] (heading-increment)
- **mado**: [MD001][mado-rules] (heading-increment)
- **panache**: [heading-hierarchy]
- **obsidian-linter**: [header-increment]

[mdl-md001]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md001.md
[rumdl-md001]: https://rumdl.dev/md001/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[heading-hierarchy]:
  https://panache.bz/reference/linter-rules.html#heading-hierarchy
[header-increment]:
  https://platers.github.io/obsidian-linter/settings/heading-rules/#header-increment
