---
id: MDS027
name: cross-file-reference-integrity
status: ready
description: Links to local files and heading anchors must resolve.
---
# MDS027: cross-file-reference-integrity

Links to local files and heading anchors must resolve.

## Settings

| Setting        | Type | Default | Description                                                                                                                |
|----------------|------|---------|----------------------------------------------------------------------------------------------------------------------------|
| `include`      | list | `[]`    | glob patterns to include                                                                                                   |
| `exclude`      | list | `[]`    | glob patterns to skip                                                                                                      |
| `strict`       | bool | `false` | check non-Markdown file links                                                                                              |
| `placeholders` | list | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Useful tokens: `var-token`, `heading-question`, `placeholder-section`.

With `strict: false`, only Markdown targets (`.md`, `.markdown`)
are checked. External links (`http:`, `https:`, `mailto:`) are
always ignored.

## Config

```yaml
rules:
  cross-file-reference-integrity:
    include:
      - "docs/**"
    exclude:
      - "docs/generated/**"
    strict: false
```

Disable:

```yaml
rules:
  cross-file-reference-integrity: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Cross File Links

See [guide](good/guide.md#overview).

Jump to the [local section](#local-anchor).

## Local Anchor

Anchor target.
```

<?/include?>

### Good -- guide target

<?include
file: good/guide.md
wrap: markdown
?>

```markdown
# Guide

## Overview

This file is a valid link target.
```

<?/include?>

### Bad -- missing file

<?include
file: bad/missing-file.md
wrap: markdown
?>

```markdown
# Broken File Link

See [guide](bad/missing.md).
```

<?/include?>

### Bad -- missing anchor

<?include
file: bad/missing-anchor.md
wrap: markdown
?>

```markdown
# Broken Heading Link

See [guide](bad/ref/guide.md#missing-section).
```

<?/include?>

## Diagnostics

| Condition       | Message                                                          |
|-----------------|------------------------------------------------------------------|
| missing file    | broken link target "x.md" not found                              |
| missing heading | broken link target "x.md#section" has no matching heading anchor |

## Meta-Information

- **ID**: MDS027
- **Name**: `cross-file-reference-integrity`
- **Status**: ready
- **Default**: enabled, include: [], exclude: [], strict: false
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: link

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
