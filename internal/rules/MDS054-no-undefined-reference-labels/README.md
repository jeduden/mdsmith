---
id: MDS054
name: no-undefined-reference-labels
status: ready
description: Reference-style links and images must have a matching link reference definition in the same file.
nature: structure
maintainability: null
---
# MDS054: no-undefined-reference-labels

Reference-style links and images must have a matching link reference
definition in the same file.

## Settings

| Setting        | Type   | Default     | Description                                                                                                                |
|----------------|--------|-------------|----------------------------------------------------------------------------------------------------------------------------|
| `shortcut`     | string | `heuristic` | Controls when bare `[label]` shortcut references are checked: `heuristic`, `always`, or `collapsed-only`.                  |
| `placeholders` | list   | `[]`        | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

### `shortcut` values

| Value            | Behaviour                                                                                                                                                                      |
|------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `heuristic`      | Flag bare `[label]` only when the label has no spaces and contains a digit, hyphen, or underscore. Image shortcuts (`![label]`) are always checked regardless of this setting. |
| `always`         | Flag every bare `[label]` whose definition is missing.                                                                                                                         |
| `collapsed-only` | Only flag `[text][label]` (full) and `[label][]` (collapsed) forms; never bare `[label]`.                                                                                      |

## Config

```yaml
rules:
  no-undefined-reference-labels:
    shortcut: heuristic
    placeholders: []
```

Disable:

```yaml
rules:
  no-undefined-reference-labels: false
```

## Examples

### Good

<?include
file: good/full-reference.md
wrap: markdown
?>

```markdown
# Full Reference

See [example][site] for more.

[site]: https://example.com
```

<?/include?>

### Good -- collapsed reference

<?include
file: good/collapsed-reference.md
wrap: markdown
?>

```markdown
# Collapsed Reference

See [example][] for more.

[example]: https://example.com
```

<?/include?>

### Good -- prose brackets skipped by heuristic

<?include
file: good/prose-brackets.md
wrap: markdown
?>

```markdown
# Prose Brackets

Use [just brackets] in prose without a definition; the heuristic
skips it because the label has spaces.
```

<?/include?>

### Bad -- undefined full reference

<?include
file: bad/undefined-full-reference.md
wrap: markdown
?>

```markdown
# Undefined Full Reference

See [example][broken] for more.
```

<?/include?>

### Bad -- undefined collapsed reference

<?include
file: bad/undefined-collapsed-reference.md
wrap: markdown
?>

```markdown
# Undefined Collapsed Reference

See [broken][] for more.
```

<?/include?>

### Bad -- undefined shortcut (heuristic)

<?include
file: bad/undefined-shortcut-heuristic.md
wrap: markdown
?>

```markdown
# Shortcut Heuristic

See [plan128] for the plan.
```

<?/include?>

## Diagnostics

| Condition                     | Message                                                       |
|-------------------------------|---------------------------------------------------------------|
| undefined full reference      | reference label "X" has no matching link reference definition |
| undefined collapsed reference | reference label "X" has no matching link reference definition |
| undefined shortcut (flagged)  | reference label "X" has no matching link reference definition |

## Background

goldmark only constructs an `*ast.Link` for a reference-style usage
when a matching link reference definition exists. When the definition
is missing, goldmark leaves the bracketed text as plain text. A
source-level scan is required to detect these dropped patterns.

Reference labels are CommonMark-normalized before lookup: case-folded,
inner whitespace collapsed, and ends trimmed. `[Foo Bar][BAR]` resolves
against `[bar]: url`.

## Meta-Information

- **ID**: MDS054
- **Name**: `no-undefined-reference-labels`
- **Status**: ready
- **Default**: enabled, shortcut: heuristic, placeholders: []
- **Fixable**: no
- **Implementation**: [source](./)
- **Category**: link

## See also

- [MDS027 cross-file-reference-integrity](../MDS027-cross-file-reference-integrity/README.md)
- [Plan 107: no-reference-style](../../../plan/107_no-reference-style.md)
- [Plan 129: no-unused-link-definitions](../../../plan/129_no-unused-link-definitions.md)
- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
