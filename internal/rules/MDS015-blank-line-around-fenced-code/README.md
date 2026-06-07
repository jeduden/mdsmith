---
id: MDS015
name: blank-line-around-fenced-code
status: ready
description: Fenced code blocks must have a blank line before and after.
category: code
nature: style
maintainability: null
markdownlint:
  - id: MD031
    name: blanks-around-fences
    partial: false
    default: true
rumdl:
  - id: MD031
    name: blanks-around-fences
    partial: false
    default: true
mado:
  - id: MD031
    name: blanks-around-fences
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: empty-line-around-code-fences
    name: empty-line-around-code-fences
    partial: false
    default: false
---
# MDS015: blank-line-around-fenced-code

Fenced code blocks must have a blank line before and after.

## Config

Enable:

```yaml
rules:
  blank-line-around-fenced-code: true
```

Disable:

```yaml
rules:
  blank-line-around-fenced-code: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

````markdown
# Title

Content here.
```go
fmt.Println("hello")
```
````

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Title

```go
fmt.Println("hello")
```

Content here.
````

<?/include?>

## Diagnostics

| Message                                                | Condition                                  |
| ------------------------------------------------------ | ------------------------------------------ |
| `fenced code block should be preceded by a blank line` | Previous line is not blank                 |
| `fenced code block should be followed by a blank line` | Next line after closing fence is not blank |

## Meta-Information

- **ID**: MDS015
- **Name**: `blank-line-around-fenced-code`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: code
- **markdownlint**: [MD031][mdl-md031] (blanks-around-fences)
- **rumdl**: [MD031][rumdl-md031] (blanks-around-fences)
- **mado**: [MD031][mado-rules] (blanks-around-fences)
- **obsidian-linter**: [empty-line-around-code-fences]

[mdl-md031]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md031.md
[rumdl-md031]: https://rumdl.dev/md031/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[empty-line-around-code-fences]:
  https://platers.github.io/obsidian-linter/settings/spacing-rules/#empty-line-around-code-fences
