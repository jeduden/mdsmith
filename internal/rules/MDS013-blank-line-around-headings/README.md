---
id: MDS013
name: blank-line-around-headings
status: ready
description: Headings must have a blank line before and after.
category: heading
nature: style
maintainability: null
markdownlint:
  - id: MD022
    name: blanks-around-headings
---
# MDS013: blank-line-around-headings

Headings must have a blank line before and after.

## Config

Enable:

```yaml
rules:
  blank-line-around-headings: true
```

Disable:

```yaml
rules:
  blank-line-around-headings: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title
## Section

Content here.
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

Content here.
```

<?/include?>

## Diagnostics

| Message                                   | Condition                            |
|-------------------------------------------|--------------------------------------|
| `heading should have a blank line before` | Previous line is not blank           |
| `heading should have a blank line after`  | Next line after heading is not blank |

## Meta-Information

- **ID**: MDS013
- **Name**: `blank-line-around-headings`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: heading
- **Markdownlint**: [MD022][mdl-md022] (blanks-around-headings)

[mdl-md022]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md022.md
