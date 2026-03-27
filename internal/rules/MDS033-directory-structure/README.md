---
id: MDS033
name: directory-structure
status: ready
description: Markdown files must exist only in explicitly allowed directories.
---
# MDS033: directory-structure

Markdown files must exist only in explicitly allowed directories.

- **ID**: MDS033
- **Name**: `directory-structure`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Key     | Type | Description                           |
|---------|------|---------------------------------------|
| allowed | list | Glob patterns for allowed directories |

## Config

Enable with allowed directories:

```yaml
rules:
  directory-structure:
    allowed:
      - "docs/**"
      - "plan/**"
      - "."
      - "internal/**/testdata/**"
```

Disable (default):

```yaml
rules:
  directory-structure: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Allowed file

This file is in an allowed directory.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Misplaced file

This file is not in an allowed directory.
```

<?/include?>
