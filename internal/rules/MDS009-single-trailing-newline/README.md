---
id: MDS009
name: single-trailing-newline
status: ready
description: File must end with exactly one newline character.
---
# MDS009: single-trailing-newline

File must end with exactly one newline character.

- **ID**: MDS009
- **Name**: `single-trailing-newline`
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
  single-trailing-newline: true
```

Disable:

```yaml
rules:
  single-trailing-newline: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

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

Content here.
```

<?/include?>
