---
id: MDS044
name: horizontal-rule-style
status: ready
description: Thematic breaks must use a consistent delimiter style, exact length, and blank-line spacing.
---
# MDS044: horizontal-rule-style

Thematic breaks must use a consistent delimiter style, exact length, and blank-line spacing.

## Settings

| Setting               | Type   | Default  | Description                                                                            |
|-----------------------|--------|----------|----------------------------------------------------------------------------------------|
| `style`               | string | `"dash"` | Delimiter character: `"dash"` (`---`), `"asterisk"` (`***`), or `"underscore"` (`___`) |
| `length`              | int    | `3`      | Exact number of delimiter characters required (minimum 3)                              |
| `require-blank-lines` | bool   | `true`   | Blank lines must appear before and after each thematic break                           |

## Config

Enable with defaults:

```yaml
rules:
  horizontal-rule-style:
    style: dash
    length: 3
    require-blank-lines: true
```

Disable:

```yaml
rules:
  horizontal-rule-style: false
```

Custom (asterisk style, length 5):

```yaml
rules:
  horizontal-rule-style:
    style: asterisk
    length: 5
```

## Examples

### Good (default settings)

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Test Document

Some text before.

---

Some text after.
```

<?/include?>

### Good (asterisk style)

<?include
file: good/asterisk.md
wrap: markdown
?>

```markdown
# Test Document

Some text before.

***

Some text after.
```

<?/include?>

### Bad (wrong delimiter)

<?include
file: bad/wrong-delimiter.md
wrap: markdown
?>

```markdown
# Test Document

Some text before.

***

Some text after.
```

<?/include?>

### Bad (internal spaces)

<?include
file: bad/internal-spaces.md
wrap: markdown
?>

```markdown
# Test Document

Some text before.

- - -

Some text after.
```

<?/include?>

### Bad (wrong length)

<?include
file: bad/wrong-length.md
wrap: markdown
?>

```markdown
# Test Document

Some text before.

-----

Some text after.
```

<?/include?>

### Bad (missing blank line above)

<?include
file: bad/no-blank-above.md
wrap: markdown
?>

```markdown
# Test Document
---

Text after.
```

<?/include?>

### Bad (missing blank line below)

<?include
file: bad/no-blank-below.md
wrap: markdown
?>

```markdown
# Test Document

---
Text after.
```

<?/include?>

## Meta-Information

- **ID**: MDS044
- **Name**: `horizontal-rule-style`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace
