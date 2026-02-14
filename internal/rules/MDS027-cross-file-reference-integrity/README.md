---
id: MDS027
name: cross-file-reference-integrity
description: Links to local files and heading anchors must resolve.
---
# MDS027: cross-file-reference-integrity

Links to local files and heading anchors must resolve.

- **ID**: MDS027
- **Name**: `cross-file-reference-integrity`
- **Default**: enabled, include: [], exclude: [], strict: false
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: link

## Settings

| Setting | Type | Default | Description                   |
|---------|------|---------|-------------------------------|
| `include` | list | `[]`      | glob patterns to include      |
| `exclude` | list | `[]`      | glob patterns to skip         |
| `strict`  | bool | `false`   | check non-Markdown file links |

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

```markdown
# Docs

See [guide](guide.md#overview).

## Overview
```

### Bad -- missing file

```markdown
# Docs

See [guide](missing.md).
```

### Bad -- missing heading

```markdown
# Docs

See [guide](guide.md#does-not-exist).
```

## Diagnostics

| Condition       | Message                                                          |
|-----------------|------------------------------------------------------------------|
| missing file    | broken link target "x.md" not found                              |
| missing heading | broken link target "x.md#section" has no matching heading anchor |
