---
id: MDS015
name: blank-line-around-fenced-code
description: Fenced code blocks must have a blank line before and after.
---
# MDS015: blank-line-around-fenced-code

Fenced code blocks must have a blank line before and after.

- **ID**: MDS015
- **Name**: `blank-line-around-fenced-code`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: code

## Config

```yaml
rules:
  blank-line-around-fenced-code: true
```

Disable:

```yaml
rules:
  blank-line-around-fenced-code: false
```

## Examples

### Bad

````markdown
Some text.
```go
code()
```
More text.
````

### Good

````markdown
Some text.

```go
code()
```

More text.
````
