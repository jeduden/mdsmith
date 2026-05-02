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

The following cases emit diagnostics but are
**not** auto-fixed:

- **Triple-delimiter runs** such as `***x***` —
  the nesting boundary is ambiguous.
- **Emphasis with a non-text first child** (e.g.
  `**[link](url)**`) — the opening delimiter
  position cannot be determined reliably; no
  diagnostic is emitted either, so these are
  silently skipped.
- **Emphasis with a non-text last child** (e.g.
  `**text [link](url)**`) — the closing delimiter
  position cannot be determined. A diagnostic is
  still reported; only the fix is suppressed.
- **`forbid-mixed-nesting` only** — when neither
  `bold` nor `italic` is configured, mixed-nesting
  diagnostics are emitted but the rule has no
  replacement delimiter to apply, so `fix` leaves
  those diagnostics unchanged.

## Meta-Information

- **ID**: MDS042
- **Name**: `emphasis-style`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes (with exceptions; see Auto-fix)
- **Implementation**:
  [source](./)
- **Category**: meta
