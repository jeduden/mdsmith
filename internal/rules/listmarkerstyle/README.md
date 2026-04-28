# MDS045 - list-marker-style

Enforce consistent bullet character for unordered lists.

## Rationale

CommonMark accepts three different characters for unordered list
markers: `-`, `*`, and `+`. When a codebase mixes these markers,
diffs become noisy (changing one marker to another shows up as a
change even when the content is identical), and readers may wonder
whether the different markers carry semantic meaning.

This rule pins a single marker character project-wide, eliminating
the three-way ambiguity and keeping diffs focused on content
changes.

## Default Configuration

```yaml
rules:
  list-marker-style:
    enabled: false  # opt-in
    style: dash     # dash | asterisk | plus
    nested: []      # optional list for depth rotation
```

The rule is disabled by default. Users must explicitly enable it and
choose a style.

## Settings

### `style`

The marker character to use for all unordered lists (when `nested`
is empty).

- `dash`: Use `-`
- `asterisk`: Use `*`
- `plus`: Use `+`

### `nested`

Optional list of style names to cycle through by depth. When set,
the marker at depth _n_ is `nested[n % len(nested)]`. Depth 0 is
the outermost list.

Example rotating between dash and asterisk:

```yaml
rules:
  list-marker-style:
    nested:
      - dash
      - asterisk
```

With this configuration:
- Depth 0 (outer) lists use `-`
- Depth 1 (nested once) lists use `*`
- Depth 2 (nested twice) lists use `-` again
- And so on

When `nested` is empty (the default), all lists use `style`
regardless of depth.

## Examples

### Good (style: dash)

```markdown
- First item
- Second item
- Third item
```

### Bad (style: dash)

```markdown
* First item
* Second item
* Third item
```

The list uses `*` but the configured style is `dash`. The rule will
flag this and auto-fix to `-`.

### Good (nested: [dash, asterisk])

```markdown
- Outer item
  * Inner item
  * Another inner item
- Another outer item
```

### Bad (nested: [dash, asterisk])

```markdown
- Outer item
  - Inner item (should be asterisk)
  - Another inner item
- Another outer item
```

The inner list at depth 1 should use `*` but uses `-`.

## Interaction with Other Rules

- **MDS016 (list-indent)**: Enforces proper indentation of nested
  lists. MDS045 only controls the marker character, not spacing.
- **MDS046 (ordered-list-numbering)**: Applies to ordered lists
  (numbered), while MDS045 applies only to unordered lists (bulleted).

## Auto-fix

The rule replaces the marker byte at the start of each list item.
Because all three markers (`-`, `*`, `+`) are single bytes, the
fix does not affect column alignment or indentation.

## Category

`list`
