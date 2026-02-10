---
id: TM006
name: no-trailing-spaces
description: No trailing whitespace at the end of lines.
---
# TM006: no-trailing-spaces

No trailing whitespace at the end of lines.

- **ID**: TM006
- **Name**: `no-trailing-spaces`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/notrailingspaces/)

## Config

```yaml
rules:
  no-trailing-spaces: true
```

## Examples

### Bad

```markdown
Some text with trailing spaces.···
```

### Good

```markdown
Some text without trailing spaces.
```
