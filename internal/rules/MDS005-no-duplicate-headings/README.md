---
id: MDS005
name: no-duplicate-headings
status: ready
description: No two headings should have the same text.
category: heading
nature: structure
maintainability: null
markdownlint:
  - id: MD024
    name: no-duplicate-heading
    default: true
rumdl:
  - id: MD024
    name: multiple-headings
    default: true
mado:
  - id: MD024
    name: no-duplicate-heading
    default: true
panache: []
---
# MDS005: no-duplicate-headings

No two headings should have the same text.

## Config

Enable:

```yaml
rules:
  no-duplicate-headings: true
```

Disable:

```yaml
rules:
  no-duplicate-headings: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

## Section

## Section
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

## Section One

Body one.

## Section Two

Body two.
```

<?/include?>

## Meta-Information

- **ID**: MDS005
- **Name**: `no-duplicate-headings`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading
- **Markdownlint**: [MD024][mdl-md024] (no-duplicate-heading)

[mdl-md024]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md024.md
