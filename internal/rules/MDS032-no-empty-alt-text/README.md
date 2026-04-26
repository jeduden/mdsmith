---
id: MDS032
name: no-empty-alt-text
status: ready
description: Images must have non-empty alt text for accessibility.
---
# MDS032: no-empty-alt-text

Images must have non-empty alt text for accessibility.

## Config

Enable (default):

```yaml
rules:
  no-empty-alt-text: true
```

Disable:

```yaml
rules:
  no-empty-alt-text: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

![A sunset over the ocean](good/image.png)
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

![](bad/image.png)
```

<?/include?>

## Meta-Information

- **ID**: MDS032
- **Name**: `no-empty-alt-text`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: accessibility
