---
id: MDS008
name: no-multiple-blanks
status: ready
description: No more than one consecutive blank line.
category: whitespace
nature: style
maintainability: null
markdownlint:
  - id: MD012
    name: no-multiple-blanks
    partial: false
    default: true
rumdl:
  - id: MD012
    name: no-multiple-blanks
    partial: false
    default: true
mado:
  - id: MD012
    name: no-multiple-blanks
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: consecutive-blank-lines
    name: consecutive-blank-lines
    partial: false
    default: false
gomarklint:
  - id: no-multiple-blank-lines
    name: no-multiple-blank-lines
    partial: false
    default: true
---
# MDS008: no-multiple-blanks

No more than one consecutive blank line.

## Config

Enable:

```yaml
rules:
  no-multiple-blanks: true
```

Disable:

```yaml
rules:
  no-multiple-blanks: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title


Two blank lines above.
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Title

One blank line above.

```text
code


more code with blank lines above
```
````

<?/include?>

## Meta-Information

- **ID**: MDS008
- **Name**: `no-multiple-blanks`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace
- **markdownlint**: [MD012][mdl-md012] (no-multiple-blanks)
- **rumdl**: [MD012][rumdl-md012] (no-multiple-blanks)
- **mado**: [MD012][mado-rules] (no-multiple-blanks)
- **obsidian-linter**: [consecutive-blank-lines]
- **gomarklint**: [no-multiple-blank-lines][gomarklint-rules]

[mdl-md012]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md012.md
[rumdl-md012]: https://rumdl.dev/md012/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[consecutive-blank-lines]:
  https://platers.github.io/obsidian-linter/settings/spacing-rules/#consecutive-blank-lines
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
