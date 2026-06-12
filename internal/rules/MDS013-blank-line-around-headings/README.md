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
    partial: false
    default: true
rumdl:
  - id: MD022
    name: blanks-around-headings
    partial: false
    default: true
mado:
  - id: MD022
    name: blanks-around-headings
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: heading-blank-lines
    name: heading-blank-lines
    partial: false
    default: false
gomarklint:
  - id: blanks-around-headings
    name: blanks-around-headings
    partial: false
    default: true
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
| ----------------------------------------- | ------------------------------------ |
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
- **markdownlint**: [MD022][mdl-md022] (blanks-around-headings)
- **rumdl**: [MD022][rumdl-md022] (blanks-around-headings)
- **mado**: [MD022][mado-rules] (blanks-around-headings)
- **obsidian-linter**: [heading-blank-lines]
- **gomarklint**: [blanks-around-headings][gomarklint-rules]

[mdl-md022]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md022.md
[rumdl-md022]: https://rumdl.dev/md022/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[heading-blank-lines]:
  https://platers.github.io/obsidian-linter/settings/spacing-rules/#heading-blank-lines
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
