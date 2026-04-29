---
id: MDS045
name: list-marker-style
status: ready
description: Unordered list items must use the configured bullet marker character.
---
# MDS045: list-marker-style

Unordered list items must use the configured bullet marker character.

## Settings

| Setting  | Type         | Default  | Description                                               |
|----------|--------------|----------|-----------------------------------------------------------|
| `style`  | string       | `"dash"` | `"dash"` (`-`), `"asterisk"` (`*`), or `"plus"` (`+`)     |
| `nested` | list(string) | `[]`     | Per-depth style rotation; cycles by `depth % len(nested)` |

## Config

Enable with dash style (the default when enabled):

```yaml
rules:
  list-marker-style:
    style: dash
```

Disable (default):

```yaml
rules:
  list-marker-style: false
```

Depth-based rotation (outer uses `-`, inner uses `*`):

```yaml
rules:
  list-marker-style:
    nested:
      - dash
      - asterisk
```

## Examples

### Good -- dash style

<?include
file: good/dash.md
wrap: markdown
?>

```markdown
# Good dash marker

This file uses dash markers consistently.

- First item
- Second item
- Third item

Nested list:

- Outer item
  - Inner item
  - Another inner item
- Another outer item
```

<?/include?>

### Good -- nested rotation

<?include
file: good/nested.md
wrap: markdown
?>

```markdown
# Good nested with rotation

Outer lists use dash, inner use asterisk.

- Outer item
  * Inner item
  * Another inner item
- Another outer item
  * More inner
    - Depth 2 cycles back to dash
      * Depth 3 cycles to asterisk
```

<?/include?>

### Bad -- wrong marker

<?include
file: bad/asterisk-with-dash.md
wrap: markdown
?>

```markdown
# Bad asterisk with dash config

This list uses asterisks but dash is configured.

* First item
* Second item
* Third item
```

<?/include?>

### Bad -- wrong nested marker

<?include
file: bad/nested-wrong.md
wrap: markdown
?>

```markdown
# Bad nested with wrong inner marker

Outer uses dash (correct), inner should use asterisk but uses dash.

- Outer item
  - Inner item should be asterisk
  - Another inner item should be asterisk
- Another outer item
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
