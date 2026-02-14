---
id: MDS008
name: no-multiple-blanks
description: No more than one consecutive blank line.
---
# MDS008: no-multiple-blanks

No more than one consecutive blank line.

- **ID**: MDS008
- **Name**: `no-multiple-blanks`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace

## Config

```yaml
rules:
  no-multiple-blanks: true
```

Disable:

```yaml
rules:
  no-multiple-blanks: false
```

## Examples

### Bad

```markdown
First paragraph.



Second paragraph.
```

### Good

```markdown
First paragraph.

Second paragraph.
```
