---
id: MDS032
name: no-empty-alt-text
status: ready
description: Images must have non-empty alt text for accessibility.
---
# MDS032: no-empty-alt-text

Images must have non-empty alt text for accessibility.

- **ID**: MDS032
- **Name**: `no-empty-alt-text`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: accessibility

## Config

Enable:

```yaml
rules:
  no-empty-alt-text: true
```

Disable:

```yaml
rules:
  no-empty-alt-text: false
```

## Examples

### Bad

```markdown
![](image.png)
```

### Good

```markdown
![A sunset over the ocean](image.png)
```
