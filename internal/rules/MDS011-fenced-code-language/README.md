---
id: MDS011
name: fenced-code-language
status: ready
description: Fenced code blocks must specify a language.
---
# MDS011: fenced-code-language

Fenced code blocks must specify a language.

## Config

Enable:

```yaml
rules:
  fenced-code-language: true
```

Disable:

```yaml
rules:
  fenced-code-language: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

````markdown
# Title

```
some code
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
````

<?/include?>

## Meta-Information

- **ID**: MDS011
- **Name**: `fenced-code-language`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: code
