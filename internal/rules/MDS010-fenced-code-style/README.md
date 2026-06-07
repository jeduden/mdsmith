---
id: MDS010
name: fenced-code-style
status: ready
description: Fenced code blocks must use a consistent delimiter.
category: code
nature: style
maintainability: null
markdownlint:
  - id: MD048
    name: code-fence-style
    partial: false
    default: true
rumdl:
  - id: MD048
    name: code-fence-style
    partial: false
    default: true
mado: []
panache: []
obsidian-linter: []
---
# MDS010: fenced-code-style

Fenced code blocks must use a consistent delimiter.

## Settings

| Setting | Type   | Default      | Description                                   |
| ------- | ------ | ------------ | --------------------------------------------- |
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
- **markdownlint**: [MD048][mdl-md048] (code-fence-style)
- **rumdl**: [MD048][rumdl-md048] (code-fence-style)

[mdl-md048]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md048.md
[rumdl-md048]: https://rumdl.dev/md048/
