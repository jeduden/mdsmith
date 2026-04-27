---
id: 109
title: List marker style rule
status: "🔲"
summary: >-
  New rule MDS043 that pins one of `-`, `*`, or `+`
  as the bullet for unordered lists. Removes the
  three-way ambiguity called out in "Exhibit C" of
  the bgslabs.org rant.
model: sonnet
---
# List marker style rule

## Goal

Let users pin a single bullet character for unordered
lists. CommonMark accepts `-`, `*`, and `+`
interchangeably. Mixed markers in one corpus produce
noisy diffs and surprise readers when nested lists
flip styles. This rule pins the marker globally.

## Background

### What goldmark exposes

Unordered lists are `*ast.List` with `IsOrdered() ==
false`. The marker character is on the node as
`Marker` (a `byte`). Each `*ast.ListItem` carries
the same marker.

### Interaction with MDS016

MDS016 (list-indent) enforces nesting indentation,
not marker choice. The two rules can fire on the
same list independently.

### Nested-list option

Some style guides want nested lists to use a
*different* marker for visual distinction (e.g. `-`
at the top level, `*` at the next). The rule
supports this with `nested:` as an ordered list of
markers cycled by depth. The default is to use the
same marker at every depth.

## Design

### Configuration

```yaml
rules:
  list-marker-style:
    style: dash          # dash | asterisk | plus
    nested: []           # optional [dash, asterisk]
                         # cycles by depth
```

Category: `list`. Disabled by default (opt-in).

### Flavor activation

[Plan 112](112_flavor-profiles.md) ships profiles
that auto-enable this rule:

- `profile: portable` activates with
  `style: dash` and empty `nested`.
- `profile: github` activates with the
  same defaults.
- `profile: plain` activates with the same
  defaults.

User overrides on top of the profile still win via
deep-merge.

### Detection

Walk `*ast.List` with `!IsOrdered()`. For each list:

1. Compute the depth — the count of `*ast.List`
   ancestors.
2. Determine the expected marker. When `nested` is
   empty, use `style`. Otherwise use
   `nested[depth % len(nested)]`.
3. If `list.Marker` differs, emit one diagnostic per
   list (not per item) at the first item's line.

### Auto-fix

Replace the marker byte at each item's start. The
indent column does not change because all three
markers are one byte wide.

### Error messages

```text
unordered list uses {actual}; configured style is {expected}
unordered list at depth {n} uses {actual}; expected {expected}
```

## Tasks

1. Scaffold `internal/rules/listmarkerstyle/`.
2. Implement `Check()` walking `*ast.List` and
   computing depth.
3. Implement `rule.Configurable` for `style` and
   `nested`. Document `nested` as replace-mode.
4. Implement `Fix()` replacing the marker byte at
   each item start.
5. Register as MDS043 in category `list`.
6. Add fixture tests covering each marker choice,
   mixed markers in one list, nested lists with and
   without `nested` set, and ordered lists (must
   not flag).
7. Add rule README.

## Acceptance Criteria

- [ ] `- item` with `style: dash` emits no diagnostic.
- [ ] `* item` with `style: dash` emits one
      diagnostic and fixes to `- item`.
- [ ] `+ item` with `style: dash` emits one
      diagnostic and fixes to `- item`.
- [ ] A nested list using `*` inside a `-` parent
      emits no diagnostic when
      `nested: [dash, asterisk]`.
- [ ] A nested list using `-` inside a `-` parent
      emits one diagnostic when
      `nested: [dash, asterisk]`.
- [ ] Ordered lists (`1. item`) emit no diagnostic.
- [ ] Rule is disabled by default.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
