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
- **Markdownlint**: [MD010][mdl-md010] (no-hard-tabs)

[mdl-md010]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md010.md
