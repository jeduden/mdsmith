---
id: TM015
name: blank-line-around-fenced-code
description: Fenced code blocks must have a blank line before and after.
---
# TM015: blank-line-around-fenced-code

Fenced code blocks must have a blank line before and after.

- **ID**: TM015
- **Name**: `blank-line-around-fenced-code`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/blanklinearoundfencedcode/)

## Config

```yaml
rules:
  blank-line-around-fenced-code: true
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
