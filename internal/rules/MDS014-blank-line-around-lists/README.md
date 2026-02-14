---
id: MDS014
name: blank-line-around-lists
description: Lists must have a blank line before and after.
---
# MDS014: blank-line-around-lists

Lists must have a blank line before and after.

- **ID**: MDS014
- **Name**: `blank-line-around-lists`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: list

## Config

Enable:

```yaml
rules:
  blank-line-around-lists: true
```

Disable:

```yaml
rules:
  blank-line-around-lists: false
```

## Examples

### Bad

```markdown
Some text.
- Item 1
- Item 2
More text.
```

### Good

```markdown
Some text.

- Item 1
- Item 2

More text.
```

## Diagnostics

| Message                                 | Condition                  |
|-----------------------------------------|----------------------------|
| `list should be preceded by a blank line` | Previous line is not blank |
| `list should be followed by a blank line` | Next line is not blank     |
