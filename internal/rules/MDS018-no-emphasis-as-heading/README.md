---
id: MDS018
name: no-emphasis-as-heading
status: ready
description: Don't use bold or emphasis on a standalone line as a heading substitute.
category: heading
nature: content
maintainability: null
markdownlint:
  - id: MD036
    name: no-emphasis-as-heading
---
# MDS018: no-emphasis-as-heading

Don't use bold or emphasis on a standalone line as a heading substitute.

## Settings

| Setting        | Type | Default | Description                                                                                                                |
|----------------|------|---------|----------------------------------------------------------------------------------------------------------------------------|
| `placeholders` | list | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Useful tokens: `var-token`, `heading-question`, `placeholder-section`.

## Config

Enable:

```yaml
rules:
  no-emphasis-as-heading: true
```

Disable:

```yaml
rules:
  no-emphasis-as-heading: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

**Bold line as heading**
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

This is a normal paragraph.
```

<?/include?>

### Good -- bold inside a table cell

Emphasis inside a pipe-table cell is intentional inline styling (a
row-label stub, a bold flag column) rather than a heading substitute.
MDS018 defers to the table-format rule and leaves these alone.

<?include
file: good/in-table-cell.md
wrap: markdown
?>

```markdown
# Table With Bold Stub

| Stub      | Value |
| --------- | ----- |
| **Alpha** | 1     |
| **Beta**  | 2     |
```

<?/include?>

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)

## Meta-Information

- **ID**: MDS018
- **Name**: `no-emphasis-as-heading`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading
- **Markdownlint**: [MD036][mdl-md036] (no-emphasis-as-heading)

[mdl-md036]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md036.md
