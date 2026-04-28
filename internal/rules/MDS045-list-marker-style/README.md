---
id: MDS045
name: list-marker-style
status: ready
description: >-
  Unordered list marker must be a single consistent
  character: dash, asterisk, or plus.
---
# MDS045: list-marker-style

Unordered list marker must be a single consistent
character: dash, asterisk, or plus.

## Settings

| Key    | Type         | Description                                     |
|--------|--------------|-------------------------------------------------|
| style  | string       | Required marker: `dash`, `asterisk`, or `plus`. |
| nested | list(string) | Marker per depth level, cycled. Replace-mode.   |

When `nested` is set, the marker at depth `d` is
`nested[d % len(nested)]`. When `nested` is empty,
`style` applies at every depth.

## Config

Enable with a target marker style:

```yaml
rules:
  list-marker-style:
    style: dash
```

Enable with per-depth cycling:

```yaml
rules:
  list-marker-style:
    style: dash
    nested: [dash, asterisk]
```

Disable (default):

```yaml
rules:
  list-marker-style: false
```

## Examples

### Good

<?include
file: good/dash.md
wrap: markdown
?>

```markdown
# Title

- item one
- item two
- item three
```

<?/include?>

### Bad

<?include
file: bad/asterisk-style-dash.md
wrap: markdown
?>

```markdown
# Title

* item one
* item two
```

<?/include?>

## Meta-Information

- **ID**: MDS045
- **Name**: `list-marker-style`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: list
