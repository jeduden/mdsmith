---
id: MDS065
name: code-block-style
status: ready
description: >-
  Code blocks must use a single delimiter — fenced or
  four-space indented — consistently across the file.
nature: style
category: code
maintainability: null
markdownlint:
  - id: MD046
    name: code-block-style
    default: true
rumdl:
  - id: MD046
    name: code-block-style
    default: true
mado:
  - id: MD046
    name: code-block-style
    default: true
panache: []
---
# MDS065: code-block-style

Code blocks must use a single delimiter — fenced or
four-space indented — consistently across the file.

## Settings

| Setting | Type   | Default    | Description                                                                                       |
| ------- | ------ | ---------- | ------------------------------------------------------------------------------------------------- |
| `style` | string | `"fenced"` | `"consistent"` (first block sets the convention), `"fenced"` (```), or `"indented"` (four-space). |

## Autofix

`mdsmith fix` converts each indented code block to a fenced
block when the resolved style is `fenced`. The converted block
gets the language tag `text` so the result satisfies
`fenced-code-language` (MDS011); edit it to the real language
after fixing.

The new fence is sized to clear any backtick run at the start
of a content line, so the embedded run never closes the
converted block prematurely. A block with a `` ``` `` line is
wrapped in a 4-backtick fence; a block with `` ```` `` gets a
5-backtick fence; and so on.

The reverse direction (fenced → indented) is **not**
auto-applied: it would drop any language tag set on the fenced
block.

## Config

Enable (default — `fenced` style):

```yaml
rules:
  code-block-style:
    style: fenced
```

Disable:

```yaml
rules:
  code-block-style: false
```

Require four-space indented blocks:

```yaml
rules:
  code-block-style:
    style: indented
```

Pin the style to whichever the first block uses:

```yaml
rules:
  code-block-style:
    style: consistent
```

## Examples

### Good — default style

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

### Good — `style: indented`

<?include
file: good/indented.md
wrap: markdown
?>

```markdown
# Title

Some text first.

    indented code
```

<?/include?>

### Bad — default style

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Some text first.

    indented code
```

<?/include?>

## Meta-Information

- **ID**: MDS065
- **Name**: `code-block-style`
- **Status**: ready
- **Default**: enabled, style: fenced
- **Fixable**: indented → fenced only
- **Implementation**:
  [source](./)
- **Category**: code
- **Markdownlint**: [MD046][mdl-md046] (code-block-style)

[mdl-md046]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md046.md
