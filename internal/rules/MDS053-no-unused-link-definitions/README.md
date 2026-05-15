---
id: MDS053
name: no-unused-link-definitions
status: ready
description: >-
  Every `[label]: url` definition must be consumed by at least one
  reference-style link or image; duplicate labels are flagged.
nature: structure
maintainability: null
---
# MDS053: no-unused-link-definitions

Every `[label]: url` definition must be consumed by at least one
reference-style link or image; duplicate labels are flagged.

An unused definition is dead weight. It survives renames, accumulates over
time, and masks broken links because `mdsmith check` never visits the URL
unless a `*ast.Link` node anchors [MDS027][mds027] to it.

CommonMark renderers silently ignore a duplicate definition — the first wins.
The second copy is invisible noise.

## Settings

| Setting          | Type | Default | Description                                                                                                                   |
|------------------|------|---------|-------------------------------------------------------------------------------------------------------------------------------|
| `ignored-labels` | list | `[]`    | Normalized labels that are never flagged as unused or duplicate. Replace-mode: a later config layer replaces the entire list. |

## Config

```yaml
rules:
  no-unused-link-definitions:
    ignored-labels:
      - comment
```

Disable:

```yaml
rules:
  no-unused-link-definitions: false
```

## Examples

### Good

<?include
file: good/used-full.md
wrap: markdown
?>

```markdown
# Used Link

See [example][ex] for more.

[ex]: https://example.com
```

<?/include?>

### Bad -- unused definition

<?include
file: bad/unused-definition.md
wrap: markdown
?>

```markdown
# Unused

Some plain prose with no links.

[orphan]: https://example.com
```

<?/include?>

### Bad -- duplicate definition

<?include
file: bad/duplicate-definition.md
wrap: markdown
?>

```markdown
# Duplicate

See [foo].

[foo]: https://first.com

[foo]: https://second.com
```

<?/include?>

## Diagnostics

| Condition            | Message                                                                |
|----------------------|------------------------------------------------------------------------|
| unused definition    | `unused link reference definition "label"`                             |
| duplicate definition | `duplicate link reference definition "label"; first defined on line N` |

## Auto-fix

Removes the offending definition line. When the line is preceded by a blank
line AND also followed by a blank line (or is the last line in the file),
the preceding blank line is also removed so removal does not leave a
double-blank behind. When only the preceding blank line is present (no
following blank), it is preserved so adjacent paragraphs remain separated.
Ignored labels are never removed.

## Meta-Information

- **ID**: MDS053
- **Name**: `no-unused-link-definitions`
- **Status**: ready
- **Default**: enabled, ignored-labels: []
- **Fixable**: yes
- **Implementation**: [source](./)
- **Category**: link

## See also

- [MDS027 cross-file-reference-integrity][mds027]
- [Plan 107: no-reference-style][plan107]
- [Plan 128: no-undefined-reference-labels][plan128]

[mds027]: ../MDS027-cross-file-reference-integrity/README.md
[plan107]: ../../../plan/107_no-reference-style.md
[plan128]: ../../../plan/128_no-undefined-reference-labels.md
