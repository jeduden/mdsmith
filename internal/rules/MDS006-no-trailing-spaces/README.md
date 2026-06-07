---
id: MDS006
name: no-trailing-spaces
status: ready
description: No trailing whitespace at the end of lines.
category: whitespace
nature: style
maintainability: null
markdownlint:
  - id: MD009
    name: no-trailing-spaces
    partial: false
    default: true
rumdl:
  - id: MD009
    name: no-trailing-spaces
    partial: false
    default: true
mado:
  - id: MD009
    name: no-trailing-spaces
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: trailing-spaces
    name: trailing-spaces
    partial: false
    default: false
---
# MDS006: no-trailing-spaces

No trailing whitespace at the end of lines.

## Config

Enable:

```yaml
rules:
  no-trailing-spaces: true
```

Disable:

```yaml
rules:
  no-trailing-spaces: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Trailing spaces here.   
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Title

No trailing spaces here.

```text
code block with language tag
```
````

<?/include?>

## Meta-Information

- **ID**: MDS006
- **Name**: `no-trailing-spaces`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace
- **markdownlint**: [MD009][mdl-md009] (no-trailing-spaces)
- **rumdl**: [MD009][rumdl-md009] (no-trailing-spaces)
- **mado**: [MD009][mado-rules] (no-trailing-spaces)
- **obsidian-linter**: [trailing-spaces]

[mdl-md009]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md009.md
[rumdl-md009]: https://rumdl.dev/md009/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[trailing-spaces]:
  https://platers.github.io/obsidian-linter/settings/spacing-rules/#trailing-spaces
