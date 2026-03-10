---
id: MDS031
name: unclosed-code-block
status: ready
description: Fenced code blocks must have a closing fence delimiter.
---
# MDS031: unclosed-code-block

Fenced code blocks must have a closing fence delimiter.

- **ID**: MDS031
- **Name**: `unclosed-code-block`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: code

## Config

Enable (default):

```yaml
rules:
  unclosed-code-block: true
```

Disable:

```yaml
rules:
  unclosed-code-block: false
```

## Examples

### Bad

`````markdown
```go
fmt.Println("hello")
// ← no closing ``` fence
`````

The opening fence has no matching closing fence, so all
following content is consumed as code.

### Good

`````markdown
```go
fmt.Println("hello")
```
`````
