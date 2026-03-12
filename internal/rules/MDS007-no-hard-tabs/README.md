---
id: MDS007
name: no-hard-tabs
status: ready
description: No tab characters. Use spaces instead.
---
# MDS007: no-hard-tabs

No tab characters. Use spaces instead.

- **ID**: MDS007
- **Name**: `no-hard-tabs`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace

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
