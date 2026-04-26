---
id: MDS022
name: max-file-length
status: ready
description: File must not exceed maximum number of lines.
---
# MDS022: max-file-length

File must not exceed maximum number of lines.

## Settings

| Setting | Type | Default | Description           |
|---------|------|---------|-----------------------|
| `max`   | int  | 300     | Maximum lines allowed |

## Config

```yaml
rules:
  max-file-length:
    max: 500
```

Disable:

```yaml
rules:
  max-file-length: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Short file within the limit.

One more line.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Short text.
Extra line.
```

<?/include?>

## Meta-Information

- **ID**: MDS022
- **Name**: `max-file-length`
- **Status**: ready
- **Default**: enabled, max: 300
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta
