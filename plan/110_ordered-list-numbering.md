---
id: 110
title: Ordered list numbering rule
status: "🔲"
summary: >-
  New rule MDS044 that pins how ordered lists number
  their items: literal sequential (`1. 2. 3.`) or all
  ones (`1. 1. 1.`). Removes the "ordered list which
  doesn't care how you ordered them" surprise from
  "Exhibit C" of the bgslabs.org rant.
model: sonnet
---
# Ordered list numbering rule

## Goal

Let users pin how ordered list items are numbered in
the source. CommonMark only reads the first item's
number and increments from there in the rendered
output. The remaining numbers are decorative — `1. 1.
1.` and `1. 7. 99.` both render as `1, 2, 3`. This
silent rewrite surprises authors and produces noisy
diffs when an editor renumbers items it did not
touch. The rule pins one of two source styles so
what the author writes matches what the reader sees.

## Background

### What goldmark exposes

Ordered lists are `*ast.List` with `IsOrdered() ==
true`. The starting number is on the node as `Start`.
Each `*ast.ListItem` has a position but goldmark
discards the literal number written by the author —
it only stores the marker character and the start.

The check must read the source line of each list
item to recover the literal number written.

### Style choices

- `sequential` — items number `1. 2. 3. ...` matching
  their position. Surprises nobody. Painful when
  inserting an item in the middle of a long list,
  because every following number shifts.
- `all-ones` — every item uses `1.` (or whichever
  number the list starts with). Insertion is free.
  The rendered output still numbers correctly.

The rant's complaint targets the *mismatch* between
source and output, not either choice on its own. A
team picks one and stops thinking about it.

### Interaction with MDS016 and MDS043

MDS016 (list-indent) handles indentation. MDS043
(list-marker-style) handles the unordered marker.
MDS044 only handles ordered numbering. The three
rules can fire on the same list independently.

## Design

### Configuration

```yaml
rules:
  ordered-list-numbering:
    style: sequential   # sequential | all-ones
    start: 1            # required first number
```

Category: `list`. Disabled by default (opt-in).

### Flavor activation

[Plan 112](112_flavor-profiles.md) ships profiles
that auto-enable this rule:

- `profile: portable` activates with
  `style: sequential` and `start: 1`.
- `profile: github` does not activate
  this rule.
- `profile: plain` activates with
  `style: sequential` and `start: 1`.

User overrides on top of the profile still win via
deep-merge.

### Detection

Walk `*ast.List` with `IsOrdered()`. For each list:

1. Read `list.Start`. If it differs from the `start`
   setting, emit one diagnostic for the first item.
2. For each item, read the source line and parse the
   leading number.
3. Compute the expected number based on `style`. For
   `sequential`, expected is `start + i`. For
   `all-ones`, expected is `start` for every item.
4. If the literal number differs from expected, emit
   one diagnostic at the item's line.

### Auto-fix

Replace the literal number on each item line with
the expected number. The marker character (`.` or
`)`) and trailing space are preserved.

Width changes (e.g. `9.` → `10.`) shift the item's
content column. The fix re-indents continuation
lines to match the new marker width so list-indent
stays consistent.

### Error messages

```text
ordered list starts at {actual}; configured start is {expected}
ordered list item {position} numbered {actual}; expected {expected}
```

## Tasks

1. Scaffold `internal/rules/orderedlistnumbering/`.
2. Implement `Check()` walking ordered `*ast.List`.
3. Implement source-line parsing for the literal
   item number.
4. Implement `rule.Configurable` for `style` and
   `start`.
5. Implement `Fix()` rewriting numbers and adjusting
   continuation indentation when marker width
   changes.
6. Register as MDS044 in category `list`.
7. Add fixture tests covering each style, the
   width-change case (single-digit to double-digit),
   wrong start, nested ordered lists, and unordered
   lists (must not flag).
8. Add rule README.

## Acceptance Criteria

- [ ] `1. a\n2. b\n3. c` with `style: sequential`
      emits no diagnostic.
- [ ] `1. a\n1. b\n1. c` with `style: sequential`
      emits two diagnostics and fixes to `1. 2. 3.`.
- [ ] `1. a\n1. b\n1. c` with `style: all-ones`
      emits no diagnostic.
- [ ] `1. a\n3. b\n7. c` with `style: all-ones`
      emits two diagnostics and fixes to all `1.`.
- [ ] `5. a\n6. b` with `start: 1` emits one
      diagnostic naming the wrong start.
- [ ] A 12-item sequential list fixes the
      single-to-double-digit boundary without
      breaking continuation indent.
- [ ] Unordered lists emit no diagnostic.
- [ ] Rule is disabled by default.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
