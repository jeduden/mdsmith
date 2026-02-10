---
id: TM018
name: no-emphasis-as-heading
description: Don't use bold or emphasis on a standalone line as a heading substitute.
---
# TM018: no-emphasis-as-heading

Don't use bold or emphasis on a standalone line as a heading substitute.

- **ID**: TM018
- **Name**: `no-emphasis-as-heading`
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](../../internal/rules/noemphasisasheading/)

## Config

```yaml
rules:
  no-emphasis-as-heading: true
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
