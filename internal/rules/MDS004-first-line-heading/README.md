---
id: MDS004
name: first-line-heading
status: ready
description: First line of the file should be a heading.
---
# MDS004: first-line-heading

First line of the file should be a heading.

## Settings

| Setting        | Type | Default | Description                                                                                                                |
|----------------|------|---------|----------------------------------------------------------------------------------------------------------------------------|
| `level`        | int  | 1       | Required heading level for the first line                                                                                  |
| `placeholders` | list | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Supported tokens: `heading-question`, `var-token`.

## Config

Enable (default):

```yaml
rules:
  first-line-heading:
    level: 1
```

Disable:

```yaml
rules:
  first-line-heading: false
```

Custom (require level 2):

```yaml
rules:
  first-line-heading:
    level: 2
```

## Examples

### Bad — content before heading

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
Some content here.

# Title
```

<?/include?>

### Bad — blank line before heading

The file below has a blank line between the frontmatter closing
`---` and `# Title`. The blank line is not visible in the code
block but triggers the diagnostic.

<?include
file: bad/blank-line.md
wrap: markdown
?>

```markdown
# Title
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Some content here.
```

<?/include?>

## Diagnostics

| Message                                                          | Condition                                           |
|------------------------------------------------------------------|-----------------------------------------------------|
| `first line should be a level {level} heading`                   | Line 1 is missing or not a heading                  |
| `first line should be a level {level} heading, found blank line` | First child is a heading but preceded by blank line |
| `first heading should be level {level}, got {n}`                 | First heading on line 1 has the wrong level         |

## Meta-Information

- **ID**: MDS004
- **Name**: `first-line-heading`
- **Status**: ready
- **Default**: enabled, level: 1
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
