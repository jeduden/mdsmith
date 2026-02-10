---
id: TM002
name: heading-style
description: Heading style must be consistent.
---
# TM002: heading-style

Heading style must be consistent.

- **ID**: TM002
- **Name**: `heading-style`
- **Default**: enabled, style: atx
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/headingstyle/)

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `style` | string | `"atx"` | `"atx"` (`# Heading`) or `"setext"` (underline with `===`/`---`) |

## Config

```yaml
rules:
  heading-style:
    style: atx
```

## Examples

### Bad (when style is `atx`)

```markdown
Heading
=======
```

### Good (when style is `atx`)

```markdown
# Heading
```
