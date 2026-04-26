---
id: MDS015
name: blank-line-around-fenced-code
status: ready
description: Fenced code blocks must have a blank line before and after.
---
# MDS015: blank-line-around-fenced-code

Fenced code blocks must have a blank line before and after.

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

<?include
file: bad/default.md
wrap: markdown
?>

````markdown
# Title

Content here.
```go
fmt.Println("hello")
```
````

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Title

```go
fmt.Println("hello")
```

Content here.
````

<?/include?>

## Diagnostics

| Message                                                | Condition                                  |
|--------------------------------------------------------|--------------------------------------------|
| `fenced code block should be preceded by a blank line` | Previous line is not blank                 |
| `fenced code block should be followed by a blank line` | Next line after closing fence is not blank |

## Meta-Information

- **ID**: MDS015
- **Name**: `blank-line-around-fenced-code`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: code
