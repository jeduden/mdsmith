---
id: MDS004
name: first-line-heading
description: First line of the file should be a heading.
---
# MDS004: first-line-heading

First line of the file should be a heading.

- **ID**: MDS004
- **Name**: `first-line-heading`
- **Default**: enabled, level: 1
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

## Settings

| Setting | Type | Default | Description                               |
|---------|------|---------|-------------------------------------------|
| `level`   | int  | 1       | Required heading level for the first line |

## Config

```yaml
rules:
  first-line-heading:
    level: 1
```

Disable:

```yaml
rules:
  first-line-heading: false
```

## Examples

### Bad

```markdown
Some introductory text.

# Heading
```

### Good

```markdown
# Heading

Some introductory text.
```
