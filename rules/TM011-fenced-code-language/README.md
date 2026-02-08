# TM011: fenced-code-language

Fenced code blocks must specify a language.

- **ID**: TM011
- **Name**: `fenced-code-language`
- **Default**: enabled
- **Fixable**: no
- **Implementation**: [`internal/rules/fencedcodelanguage/`](../../internal/rules/fencedcodelanguage/)

## Config

```yaml
rules:
  fenced-code-language: true
```

## Examples

### Bad

````markdown
```
some code without a language
```
````

### Good

````markdown
```go
fmt.Println("hello")
```
````
