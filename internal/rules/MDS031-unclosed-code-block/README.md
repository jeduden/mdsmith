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

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Properly Closed

```go
fmt.Println("hello")
```
````

<?/include?>

The opening fence has no matching closing fence, so all
following content is consumed as code.

### Bad -- empty fence

<?include
file: bad/empty.md
wrap: markdown
?>

````markdown
# Empty Unclosed Fence

```
````

<?/include?>
