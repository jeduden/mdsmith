---
id: MDS013
name: blank-line-around-headings
description: Headings must have a blank line before and after.
---
# MDS013: blank-line-around-headings

Headings must have a blank line before and after.

- **ID**: MDS013
- **Name**: `blank-line-around-headings`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: heading

## Config

```yaml
rules:
  blank-line-around-headings: true
```

Disable:

```yaml
rules:
  blank-line-around-headings: false
```

## Examples

### Bad

```markdown
Some text.
## Heading
More text.
```

### Good

```markdown
Some text.

## Heading

More text.
```
