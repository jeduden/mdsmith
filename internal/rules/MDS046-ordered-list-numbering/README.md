---
id: MDS046
name: ordered-list-numbering
status: ready
description: Ordered list items must be numbered in the configured style.
---
# MDS046: ordered-list-numbering

Ordered list items must be numbered in the configured style.

## Settings

| Setting | Type   | Default        | Description                                          |
|---------|--------|----------------|------------------------------------------------------|
| `style` | string | `"sequential"` | `"sequential"` (1. 2. 3.) or `"all-ones"` (1. 1. 1.) |
| `start` | int    | `1`            | Required first number for every ordered list         |

## Config

Enable with sequential numbering:

```yaml
rules:
  ordered-list-numbering:
    style: sequential
    start: 1
```

Disable (default):

```yaml
rules:
  ordered-list-numbering: false
```

All-ones style (insertion-friendly):

```yaml
rules:
  ordered-list-numbering:
    style: all-ones
    start: 1
```

## Examples

### Good -- sequential

<?include
file: good/sequential.md
wrap: markdown
?>

```markdown
# Title

1. first item
2. second item
3. third item
```

<?/include?>

### Good -- all-ones

<?include
file: good/all-ones.md
wrap: markdown
?>

```markdown
# Title

1. first item
1. second item
1. third item
```

<?/include?>

### Bad -- sequential style with all-ones source

<?include
file: bad/sequential-mismatch.md
wrap: markdown
?>

```markdown
# Title

1. first item
1. second item
1. third item
```

<?/include?>

### Bad -- wrong start

<?include
file: bad/wrong-start.md
wrap: markdown
?>

```markdown
# Title

5. first item
6. second item
```

<?/include?>

## Diagnostics

- `ordered list starts at {actual}; configured start is {expected}` —
  the first item's number does not match the configured `start`. Fires
  once per list, on the first item's line.
- `ordered list item {position} numbered {actual}; expected {expected}`
  — an item later in the list deviates from the expected number under
  the configured `style`.

## Edge Cases

When the fix changes a marker's digit width (for example, item 10
growing from `1.` to `10.`), the rule re-indents the item's
continuation lines and any nested blocks so the content column
still aligns with the new marker prefix. Blank lines inside the
item are left unchanged so the fix does not introduce trailing
whitespace.

Each list is checked independently. A nested ordered
list inside a list item is treated as its own list. The
outer list's style does not exempt the nested list.

When a list's first item starts at a number different
from the configured `start`, the rule emits only the
start-mismatch diagnostic for that list. Per-item
diagnostics would use the configured `start` as their
baseline, matching what auto-fix produces. Reporting
them alongside "wrong start" would name renumbering
that the start fix already covers.

## Meta-Information

- **ID**: MDS046
- **Name**: `ordered-list-numbering`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: list
