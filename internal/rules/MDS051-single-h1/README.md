---
id: MDS051
name: single-h1
status: ready
description: At most one H1 heading is allowed per file.
category: heading
nature: structure
maintainability: null
markdownlint:
  - id: MD025
    name: single-h1
    partial: false
    default: true
rumdl:
  - id: MD025
    name: single-title
    partial: false
    default: true
mado:
  - id: MD025
    name: single-h1
    partial: false
    default: true
panache: []
obsidian-linter: []
gomarklint:
  - id: single-h1
    name: single-h1
    partial: false
    default: true
---
# MDS051: single-h1

At most one H1 heading is allowed per file.

A second H1 usually indicates two documents merged into one or a
heading-level mistake.

## Settings

| Setting              | Type   | Default   | Description                                                                                                                                 |
| -------------------- | ------ | --------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `front-matter-title` | string | `"title"` | Front-matter field that counts as an H1. When the field is set and the file also contains an H1, the H1 is flagged. Set to `""` to disable. |

## Config

Enable:

```yaml
rules:
  single-h1: true
```

Disable (default):

```yaml
rules:
  single-h1: false
```

Ignore front-matter title:

```yaml
rules:
  single-h1:
    front-matter-title: ""
```

## Examples

### Bad — two H1 headings

```markdown
# First

## Section

# Second
```

### Bad — front-matter title conflicts with H1

```yaml
---
title: My Doc
---
```

```markdown
# My Doc
```

### Good — single H1

```markdown
# Title

## Section

### Sub-section
```

## Diagnostics

| Message                                             | Condition                                                                                             |
| --------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `extra H1 heading; only one H1 is allowed per file` | A second or later H1 heading exists in the document                                                   |
| `h1 heading conflicts with front-matter title`      | An H1 heading exists and the front matter contains the configured field with a non-empty string value |

## See also

- [MDS003](../MDS003-heading-increment/) — heading hierarchy (no level skips)
- [MDS004](../MDS004-first-line-heading/) — first-line heading
- [MDS005](../MDS005-no-duplicate-headings/) — no duplicate heading text

## Meta-Information

- **ID**: MDS051
- **Name**: `single-h1`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: yes — extra H1s are demoted to H2; front-matter conflicts are not auto-fixed
- **Implementation**: [source](./)
- **Category**: heading
- **markdownlint**: [MD025][mdl-md025] (single-h1)
- **rumdl**: [MD025][rumdl-md025] (single-title)
- **mado**: [MD025][mado-rules] (single-h1)
- **gomarklint**: [single-h1][gomarklint-rules]

[mdl-md025]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md025.md
[rumdl-md025]: https://rumdl.dev/md025/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
