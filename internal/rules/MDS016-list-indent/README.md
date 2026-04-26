---
id: MDS016
name: list-indent
status: ready
description: List items must use consistent indentation.
---
# MDS016: list-indent

List items must use consistent indentation.

## Settings

| Setting  | Type | Default | Description                            |
|----------|------|---------|----------------------------------------|
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
