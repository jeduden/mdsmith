---
id: MDS011
name: fenced-code-language
status: ready
description: Fenced code blocks must specify a language.
category: code
nature: style
maintainability: null
markdownlint:
  - id: MD040
    name: fenced-code-language
    partial: false
    default: true
rumdl:
  - id: MD040
    name: fenced-code-language
    partial: false
    default: true
mado:
  - id: MD040
    name: fenced-code-language
    partial: false
    default: true
panache: []
obsidian-linter: []
gomarklint:
  - id: fenced-code-language
    name: fenced-code-language
    partial: false
    default: true
---
# MDS011: fenced-code-language

Fenced code blocks must specify a language.

## Config

Enable:

```yaml
rules:
  fenced-code-language: true
```

Disable:

```yaml
rules:
  fenced-code-language: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

````markdown
# Title

```
some code
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
````

<?/include?>

## Meta-Information

- **ID**: MDS011
- **Name**: `fenced-code-language`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: code
- **markdownlint**: [MD040][mdl-md040] (fenced-code-language)
- **rumdl**: [MD040][rumdl-md040] (fenced-code-language)
- **mado**: [MD040][mado-rules] (fenced-code-language)
- **gomarklint**: [fenced-code-language][gomarklint-rules]

[mdl-md040]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md040.md
[rumdl-md040]: https://rumdl.dev/md040/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
