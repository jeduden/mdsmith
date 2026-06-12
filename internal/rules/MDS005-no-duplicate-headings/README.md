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
    partial: false
    default: true
rumdl:
  - id: MD024
    name: multiple-headings
    partial: false
    default: true
mado:
  - id: MD024
    name: no-duplicate-heading
    partial: false
    default: true
panache: []
obsidian-linter: []
gomarklint:
  - id: duplicate-heading
    name: duplicate-heading
    partial: false
    default: true
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
- **markdownlint**: [MD024][mdl-md024] (no-duplicate-heading)
- **rumdl**: [MD024][rumdl-md024] (multiple-headings)
- **mado**: [MD024][mado-rules] (no-duplicate-heading)
- **gomarklint**: [duplicate-heading][gomarklint-rules]

[mdl-md024]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md024.md
[rumdl-md024]: https://rumdl.dev/md024/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
