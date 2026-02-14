---
id: MDS010
name: fenced-code-style
description: Fenced code blocks must use a consistent delimiter.
---
# MDS010: fenced-code-style

Fenced code blocks must use a consistent delimiter.

- **ID**: MDS010
- **Name**: `fenced-code-style`
- **Default**: enabled, style: backtick
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: code

## Settings

| Setting | Type   | Default    | Description                       |
|---------|--------|------------|-----------------------------------|
| `style`   | string | `"backtick"` | `"backtick"` (`` ``` ``) or `"tilde"` (`~~~`) |

## Config

Enable (default):

```yaml
rules:
  fenced-code-style:
    style: backtick
```

Disable:

```yaml
rules:
  fenced-code-style: false
```

Custom (tilde style):

```yaml
rules:
  fenced-code-style:
    style: tilde
```

## Examples

### Bad (when style is `backtick`)

```markdown
~~~go
fmt.Println("hello")
~~~
```

### Good (when style is `backtick`)

````markdown
```go
fmt.Println("hello")
```
````
