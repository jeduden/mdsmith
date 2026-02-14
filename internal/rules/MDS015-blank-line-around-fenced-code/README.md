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

Enable:

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

## Diagnostics

| Message                                              | Condition                                  |
|------------------------------------------------------|--------------------------------------------|
| `fenced code block should be preceded by a blank line` | Previous line is not blank                 |
| `fenced code block should be followed by a blank line` | Next line after closing fence is not blank |
