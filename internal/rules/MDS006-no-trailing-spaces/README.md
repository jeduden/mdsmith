---
id: MDS006
name: no-trailing-spaces
description: No trailing whitespace at the end of lines.
---
# MDS006: no-trailing-spaces

No trailing whitespace at the end of lines.

- **ID**: MDS006
- **Name**: `no-trailing-spaces`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace

## Config

```yaml
rules:
  no-trailing-spaces: true
```

Disable:

```yaml
rules:
  no-trailing-spaces: false
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
