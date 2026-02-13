---
id: MDS009
name: single-trailing-newline
description: File must end with exactly one newline character.
---
# MDS009: single-trailing-newline

File must end with exactly one newline character.

- **ID**: MDS009
- **Name**: `single-trailing-newline`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace

## Config

```yaml
rules:
  single-trailing-newline: true
```

Disable:

```yaml
rules:
  single-trailing-newline: false
```

## Examples

### Bad

```markdown
Some text without a final newline.
```

```markdown
Some text with too many trailing newlines.

⏎
⏎
```

### Good

```markdown
Some text with exactly one trailing newline.
⏎
```
