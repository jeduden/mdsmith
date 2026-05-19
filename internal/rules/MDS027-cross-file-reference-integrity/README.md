---
id: MDS027
name: cross-file-reference-integrity
status: ready
description: Links to local files and heading anchors must resolve.
category: link
nature: structure
maintainability: null
markdownlint:
  - id: MD051
    name: link-fragments
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
are checked (except images — see `links.validate-images`).
External links (`http:`, `https:`, `mailto:`) are always ignored.

### links block

| Setting                          | Type   | Default | Description                                                                            |
|----------------------------------|--------|---------|----------------------------------------------------------------------------------------|
| `links.validate-images`          | bool   | `true`  | Check `*ast.Image` targets (e.g. `![alt](img.png)`) regardless of `strict` mode.       |
| `links.validate-reference-style` | bool   | `true`  | Check reference-style link targets (e.g. `[text][label]` / `[label]: url`).            |
| `links.site-root`                | string | `""`    | When set, resolve absolute paths (e.g. `/docs/rules/`) against this directory on disk. |

When `links.validate-images` is on, image targets are checked even
when `strict: false` — an image `![](missing.png)` is flagged as a
broken target regardless of extension. Set to `false` to restore the
pre-hardening behavior where images were silently skipped.

When `links.site-root` is unset, absolute-path links (`/foo/bar`)
are silently skipped (the behavior before this setting was added).

## Config

```yaml
rules:
  cross-file-reference-integrity:
    include:
      - "docs/**"
    exclude:
      - "docs/generated/**"
    strict: false
    links:
      site-root: ""
      validate-images: true
      validate-reference-style: true
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

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)

## Meta-Information

- **ID**: MDS027
- **Name**: `cross-file-reference-integrity`
- **Status**: ready
- **Default**: enabled, include: [], exclude: [], strict: false,
  links.validate-images: true, links.validate-reference-style: true,
  links.site-root: ""
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: link
- **Markdownlint**: [MD051][mdl-md051] (link-fragments)

[mdl-md051]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md051.md
