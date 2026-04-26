---
id: MDS010
name: fenced-code-style
status: ready
description: Fenced code blocks must use a consistent delimiter.
---
# MDS010: fenced-code-style

Fenced code blocks must use a consistent delimiter.

## Settings

| Setting | Type   | Default      | Description                                   |
|---------|--------|--------------|-----------------------------------------------|
| `style` | string | `"backtick"` | `"backtick"` (`` ``` ``) or `"tilde"` (`~~~`) |

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

### Good (when style is `backtick`)

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

### Good (when style is `tilde`)

<?include
file: good/tilde.md
wrap: markdown
?>

```markdown
# Title

~~~go
fmt.Println("hello")
~~~
```

<?/include?>

### Bad (when style is `backtick`)

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

~~~go
fmt.Println("hello")
~~~
```

<?/include?>

### Bad (when style is `tilde`)

<?include
file: bad/tilde.md
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

- **ID**: MDS010
- **Name**: `fenced-code-style`
- **Status**: ready
- **Default**: enabled, style: backtick
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: code
