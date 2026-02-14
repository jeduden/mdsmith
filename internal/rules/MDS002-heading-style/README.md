---
id: MDS002
name: heading-style
description: Heading style must be consistent.
---
# MDS002: heading-style

Heading style must be consistent.

- **ID**: MDS002
- **Name**: `heading-style`
- **Default**: enabled, style: atx
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: heading

## Settings

| Setting | Type   | Default | Description                                            |
|---------|--------|---------|--------------------------------------------------------|
| `style`   | string | `"atx"`   | `"atx"` (`# Heading`) or `"setext"` (underline with `===`/`---`) |

## Config

Enable (default):

```yaml
rules:
  heading-style:
    style: atx
```

Disable:

```yaml
rules:
  heading-style: false
```

Custom (setext style):

```yaml
rules:
  heading-style:
    style: setext
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

## Diagnostics

| Message                        | Condition                                 |
|--------------------------------|-------------------------------------------|
| `heading style should be atx`    | `style: atx` and a Setext heading is found  |
| `heading style should be setext` | `style: setext` and an ATX heading is found |
