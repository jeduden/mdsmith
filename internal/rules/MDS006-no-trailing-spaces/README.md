---
id: MDS006
name: no-trailing-spaces
status: ready
description: No trailing whitespace at the end of lines.
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
