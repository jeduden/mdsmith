---
id: MDS016
name: list-indent
status: ready
description: List items must use consistent indentation.
category: list
nature: style
maintainability: null
markdownlint:
  - id: MD005
    name: list-indent
    partial: true
    default: true
  - id: MD007
    name: ul-indent
    default: true
rumdl:
  - id: MD005
    name: list-indent
    partial: true
    default: true
  - id: MD007
    name: ul-indent
    default: true
mado:
  - id: MD005
    name: list-indent
    partial: true
    default: true
  - id: MD007
    name: ul-indent
    default: false
panache: []
---
# MDS016: list-indent

List items must use consistent indentation.

## Settings

| Setting  | Type | Default | Description                            |
| -------- | ---- | ------- | -------------------------------------- |
| `spaces` | int  | 2       | Number of spaces per indentation level |

## Config

Enable (default):

```yaml
rules:
  list-indent:
    spaces: 2
```

Disable:

```yaml
rules:
  list-indent: false
```

Custom (4-space list indent):

```yaml
rules:
  list-indent:
    spaces: 4
```

## Examples

### Bad (when spaces is 2)

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

- item one
    - nested item
```

<?/include?>

4-space indent when configured for 4 spaces:

<?include
file: bad/spaces-4.md
wrap: markdown
?>

```markdown
# Title

- item one
  - nested item
```

<?/include?>

### Good (when spaces is 2)

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

- item one
  - nested item
```

<?/include?>

4-space indent when configured for 4 spaces:

<?include
file: good/spaces-4.md
wrap: markdown
?>

```markdown
# Title

- item one
    - nested item
```

<?/include?>

## Meta-Information

- **ID**: MDS016
- **Name**: `list-indent`
- **Status**: ready
- **Default**: enabled, spaces: 2
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: list
- **Markdownlint**:
  - [MD005][mdl-md005] (list-indent) (partial)
  - [MD007][mdl-md007] (ul-indent)

[mdl-md005]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md005.md
[mdl-md007]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md007.md
