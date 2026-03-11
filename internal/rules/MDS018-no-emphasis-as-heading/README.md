---
id: MDS018
name: no-emphasis-as-heading
status: ready
description: Don't use bold or emphasis on a standalone line as a heading substitute.
---
# MDS018: no-emphasis-as-heading

Don't use bold or emphasis on a standalone line as a heading substitute.

- **ID**: MDS018
- **Name**: `no-emphasis-as-heading`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

## Config

Enable:

```yaml
rules:
  no-emphasis-as-heading: true
```

Disable:

```yaml
rules:
  no-emphasis-as-heading: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

**Bold line as heading**
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

This is a normal paragraph.
```

<?/include?>
