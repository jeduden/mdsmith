---
id: MDS008
name: no-multiple-blanks
status: ready
description: No more than one consecutive blank line.
---
# MDS008: no-multiple-blanks

No more than one consecutive blank line.

- **ID**: MDS008
- **Name**: `no-multiple-blanks`
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
