---
id: MDS044
name: horizontal-rule-style
status: ready
description: >-
  Thematic breaks must use a consistent delimiter style, exact
  length, and blank-line spacing.
category: whitespace
nature: style
maintainability: null
---
# MDS044: horizontal-rule-style

Thematic breaks must use a consistent delimiter style, exact
length, and blank-line spacing.

CommonMark accepts `---`, `***`, and `___` with any length ≥ 3
and optional internal spaces. This rule pins one delimiter,
enforces an exact length, and requires surrounding blank lines
so `---` cannot be confused with a setext heading underline.

## Settings

| Setting               | Type   | Default  | Description                                                                             |
|-----------------------|--------|----------|-----------------------------------------------------------------------------------------|
| `style`               | string | `"dash"` | Delimiter character: `"dash"` (`---`), `"asterisk"` (`***`), or `"underscore"` (`___`). |
| `length`              | int    | `3`      | Exact number of delimiter characters required (minimum 3).                              |
| `require-blank-lines` | bool   | `true`   | Blank lines must appear before and after each thematic break.                           |

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

## Diagnostics

```text
horizontal rule uses {actual}; configured style is {expected}
horizontal rule has internal spaces
horizontal rule has length {actual}; configured length is {expected}
horizontal rule needs a blank line above
horizontal rule needs a blank line below
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

## Auto-fix

The rule rewrites each thematic break to the canonical form
(`style` repeated `length` times). It also inserts any missing
blank lines above and below.

## Meta-Information

- **ID**: MDS044
- **Name**: `horizontal-rule-style`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace
