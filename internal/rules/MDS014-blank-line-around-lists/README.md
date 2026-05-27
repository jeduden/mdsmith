---
id: MDS014
name: blank-line-around-lists
status: ready
description: Lists must have a blank line before and after.
category: list
nature: style
maintainability: null
markdownlint:
  - id: MD032
    name: blanks-around-lists
    default: true
rumdl:
  - id: MD032
    name: blanks-around-lists
    default: true
mado:
  - id: MD032
    name: blanks-around-lists
    default: false
panache: []
---
# MDS014: blank-line-around-lists

Lists must have a blank line before and after.

## Config

Enable:

```yaml
rules:
  blank-line-around-lists: true
```

Disable:

```yaml
rules:
  blank-line-around-lists: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Content here.
- item one
- item two
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

- item one
- item two

Content here.
```

<?/include?>

## Diagnostics

| Message                                   | Condition                  |
| ----------------------------------------- | -------------------------- |
| `list should be preceded by a blank line` | Previous line is not blank |
| `list should be followed by a blank line` | Next line is not blank     |

## Meta-Information

- **ID**: MDS014
- **Name**: `blank-line-around-lists`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: list
- **Markdownlint**: [MD032][mdl-md032] (blanks-around-lists)

[mdl-md032]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md032.md
