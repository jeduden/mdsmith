---
id: MDS003
name: heading-increment
status: ready
description: Heading levels should increment by one. No jumping from `#` to `###`.
---
# MDS003: heading-increment

Heading levels should increment by one. No jumping from `#` to `###`.

## Settings

| Setting        | Type | Default | Description                                                                                                                |
|----------------|------|---------|----------------------------------------------------------------------------------------------------------------------------|
| `placeholders` | list | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Supported tokens: `heading-question`, `placeholder-section`.

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

## Meta-Information

- **ID**: MDS003
- **Name**: `heading-increment`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
