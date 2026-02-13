---
id: MDS018
name: no-emphasis-as-heading
description: Don't use bold or emphasis on a standalone line as a heading substitute.
---
# MDS018: no-emphasis-as-heading

Don't use bold or emphasis on a standalone line as a heading substitute.

- **ID**: MDS018
- **Name**: `no-emphasis-as-heading`
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

## Config

```yaml
rules:
  no-emphasis-as-heading: true
```

Disable:

```yaml
rules:
  no-emphasis-as-heading: false
```

## Examples

### Bad

```markdown
**This looks like a heading**

Some text below it.
```

### Good

```markdown
## This is a proper heading

Some text below it.
```
