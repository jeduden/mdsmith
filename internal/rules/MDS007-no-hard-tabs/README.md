---
id: MDS007
name: no-hard-tabs
status: ready
description: No tab characters. Use spaces instead.
category: whitespace
nature: style
maintainability: null
markdownlint:
  - id: MD010
    name: no-hard-tabs
    partial: false
    default: true
rumdl:
  - id: MD010
    name: no-hard-tabs
    partial: false
    default: true
mado:
  - id: MD010
    name: no-hard-tabs
    partial: false
    default: true
panache: []
obsidian-linter: []
gomarklint:
  - id: no-hard-tabs
    name: no-hard-tabs
    partial: false
    default: true
---
# MDS007: no-hard-tabs

No tab characters. Use spaces instead.

## Config

Enable:

```yaml
rules:
  no-hard-tabs: true
```

Disable:

```yaml
rules:
  no-hard-tabs: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

hel	lo
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Title

No tabs here.

```text
	indented with tab
	more tabbed code
```
````

<?/include?>

## Meta-Information

- **ID**: MDS007
- **Name**: `no-hard-tabs`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace
- **markdownlint**: [MD010][mdl-md010] (no-hard-tabs)
- **rumdl**: [MD010][rumdl-md010] (no-hard-tabs)
- **mado**: [MD010][mado-rules] (no-hard-tabs)
- **gomarklint**: [no-hard-tabs][gomarklint-rules]

[mdl-md010]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md010.md
[rumdl-md010]: https://rumdl.dev/md010/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
