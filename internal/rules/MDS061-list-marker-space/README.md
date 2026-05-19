---
id: MDS061
name: list-marker-space
status: ready
description: >-
  Each list marker must be followed by the configured number
  of spaces.
nature: style
category: list
maintainability: null
markdownlint:
  - id: MD030
    name: list-marker-space
---
# MDS061: list-marker-space

Each list marker must be followed by the configured number
of spaces.

## Settings

| Setting     | Type | Default | Description                                 |
|-------------|------|---------|---------------------------------------------|
| `ul-single` | int  | `1`     | Spaces after unordered marker, single items |
| `ul-multi`  | int  | `1`     | Spaces after unordered marker, multi items  |
| `ol-single` | int  | `1`     | Spaces after ordered marker, single items   |
| `ol-multi`  | int  | `1`     | Spaces after ordered marker, multi items    |

A list item is "multi" when it has more than one block-level
child (a blank-line-separated continuation paragraph, a nested
list, or a code block inside the item). Single items have one
block child.

`ul-multi` and `ol-multi` are checked but **not auto-fixed**.
Adjusting the marker gap on a multi-paragraph item requires
re-indenting every continuation line by the same delta; doing so
correctly across all block child types is left to the author.
Single-paragraph items are fixed automatically.

## Config

Enable with default settings (one space everywhere):

```yaml
rules:
  list-marker-space: true
```

Disable:

```yaml
rules:
  list-marker-space: false
```

Require two spaces after the marker for multi-paragraph items:

```yaml
rules:
  list-marker-space:
    ul-multi: 2
    ol-multi: 2
```

## Examples

### Good -- one space

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Unordered list marker space

One space after each unordered marker.

- First item
- Second item
- Third item
```

<?/include?>

### Good -- ordered list

<?include
file: good/ol.md
wrap: markdown
?>

```markdown
# Ordered list marker space

One space after each ordered marker.

1. First item
2. Second item
3. Third item
```

<?/include?>

### Bad -- two spaces

<?include
file: bad/ul-two-spaces.md
wrap: markdown
?>

```markdown
# Two spaces after unordered marker

Unordered list with two spaces after each marker.

-  First item
-  Second item
```

<?/include?>

### Bad -- ordered two spaces

<?include
file: bad/ol-two-spaces.md
wrap: markdown
?>

```markdown
# Two spaces after ordered marker

Ordered list with two spaces after each marker.

1.  First item
2.  Second item
```

<?/include?>

## Meta-Information

- **ID**: MDS061
- **Name**: `list-marker-space`
- **Status**: ready
- **Default**: enabled
- **Fixable**: single-paragraph items only
- **Implementation**:
  [source](./)
- **Category**: list
- **Markdownlint**: [MD030][mdl-md030] (list-marker-space)

[mdl-md030]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md030.md
