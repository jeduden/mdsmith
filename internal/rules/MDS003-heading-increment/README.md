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
    default: true
rumdl:
  - id: MD001
    name: heading-increment
    default: true
mado:
  - id: MD001
    name: heading-increment
    default: true
panache:
  - id: heading-hierarchy
    name: heading-hierarchy
    default: true
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
- **Markdownlint**: [MD001][mdl-md001] (heading-increment)

[mdl-md001]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md001.md
