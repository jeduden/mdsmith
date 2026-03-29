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

Patterns are matched against the full file path using
forward slashes. Use `**` to match nested directories
(e.g., `docs/**` allows any file under `docs/`). The
special pattern `"."` allows root-level files only.

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

Enabling without `allowed` emits a config warning:

```text
directory-structure: rule enabled but no "allowed" patterns configured
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
