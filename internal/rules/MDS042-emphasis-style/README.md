---
id: MDS042
name: emphasis-style
status: ready
description: >-
  Enforces a single delimiter character for bold
  and italic emphasis, and optionally forbids
  cross-delimiter nesting.
---
# MDS042: emphasis-style

Enforces a single delimiter character for bold
and italic emphasis, and optionally forbids
cross-delimiter nesting.

CommonMark accepts `**bold**`, `__bold__`,
`*italic*`, and `_italic_` interchangeably.
Mixed delimiters produce inconsistent diffs.
This rule makes the choice explicit.

## Settings

| Key                  | Type   | Default | Description                                                     |
|----------------------|--------|---------|-----------------------------------------------------------------|
| bold                 | string | `""`    | Required delimiter: `asterisk` or `underscore`.                 |
| italic               | string | `""`    | Required delimiter: `asterisk` or `underscore`.                 |
| forbid-mixed-nesting | bool   | `false` | Flag emphasis whose delimiter differs from its parent emphasis. |

Leave `bold` or `italic` empty to skip enforcement
for that role.

## Config

```yaml
rules:
  emphasis-style:
    bold: asterisk
    italic: underscore
    forbid-mixed-nesting: true
```

Disable (default):

```yaml
rules:
  emphasis-style: false
```

## Diagnostics

```text
bold uses underscore; configured style is asterisk
italic uses asterisk; configured style is underscore
mixed emphasis delimiters: underscore wraps asterisk
```

## Examples

### Good

<?include
file: good/both-styles.md
wrap: markdown
?>

```markdown
# Heading

Some **bold** and _italic_ text.
```

<?/include?>

### Bad

<?include
file: bad/underscore-bold.md
wrap: markdown
?>

```markdown
# Heading

Some __bold__ text.
```

<?/include?>

## Auto-fix

Opening and closing delimiter bytes are replaced
in the source. The fix is byte-for-byte
substitution.

Triple-delimiter runs such as `***x***` are
detected as diagnostics but skipped during
auto-fix. The nesting boundary is ambiguous.

## Meta-Information

- **ID**: MDS042
- **Name**: `emphasis-style`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes (except triple-delimiter runs)
- **Implementation**:
  [source](./)
- **Category**: meta
