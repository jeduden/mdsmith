---
id: TM009
name: single-trailing-newline
description: File must end with exactly one newline character.
---
# TM009: single-trailing-newline

File must end with exactly one newline character.

- **ID**: TM009
- **Name**: `single-trailing-newline`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/singletrailingnewline/)

## Config

```yaml
rules:
  single-trailing-newline: true
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
