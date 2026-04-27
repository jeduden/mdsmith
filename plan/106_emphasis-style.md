---
id: 106
title: Emphasis style rule
status: "🔲"
summary: >-
  New rule MDS040 that pins one delimiter per role:
  one of asterisk or underscore for bold, one for
  italic. Removes the bold/italic ambiguity called
  out as "Exhibit A" in the bgslabs.org markdown
  rant.
model: sonnet
---
# Emphasis style rule

## Goal

Let users pin a single delimiter for bold and a single
delimiter for italic. CommonMark accepts `**bold**`,
`__bold__`, `*italic*`, and `_italic_` interchangeably.
Multiple delimiters in one corpus produce inconsistent
diffs and stress every parser implementation. This rule
makes the choice explicit and enforces it.

## Background

### What goldmark exposes

Bold and italic both render to `*ast.Emphasis` nodes.
The node carries a `Level` field: `1` for italic, `2`
for bold. The chosen delimiter character does not
appear on the AST node — it must be read back from
the source via the node's first child segment, looking
at the byte immediately before the segment start.

### Mixed-nesting cases

The rant flags cross-delimiter nests like `_*bold*_`
and `*_bold_*` as ambiguous. CommonMark resolves
them deterministically, but humans cannot. A
`forbid-mixed-nesting` setting flags any Emphasis
whose delimiter differs from its parent Emphasis.

### Why a separate rule from MDS018

MDS018 (no-emphasis-as-heading) flags emphasis used
*as a heading*. MDS040 flags emphasis with the wrong
*delimiter*. The two rules can coexist on the same
file without overlap.

## Design

### Configuration

```yaml
rules:
  emphasis-style:
    bold: asterisk        # asterisk | underscore
    italic: underscore    # asterisk | underscore
    forbid-mixed-nesting: true
```

Category: `whitespace` is wrong; use a new category
or reuse `meta`. **Chosen: `meta`**, matching MDS034
and the new MDS039.

Disabled by default (opt-in) — existing corpora vary.

### Flavor activation

[Plan 112](112_flavor-profiles.md) ships profiles
that auto-enable this rule:

- `profile: portable` activates with
  `bold: asterisk`, `italic: underscore`, and
  `forbid-mixed-nesting: true`.
- `profile: github` activates with the
  same defaults.
- `profile: plain` activates with the same
  defaults. (A future `no-emphasis` rule would
  forbid `*` and `_` runs entirely under `plain`.)

User overrides on top of the profile still win via
deep-merge.

### Detection

Walk the AST. For every `*ast.Emphasis`:

1. Find the byte immediately before the first child
   segment to read the actual opening delimiter.
2. If `Level == 2` and the byte does not match the
   `bold` setting, emit one diagnostic.
3. If `Level == 1` and the byte does not match the
   `italic` setting, emit one diagnostic.
4. If `forbid-mixed-nesting` is true and the parent
   chain contains an Emphasis with a different
   delimiter, emit one diagnostic.

### Auto-fix

Replace the opening and closing delimiter bytes in
the source. Bold uses two delimiter characters per
side, italic uses one. The fix is byte-for-byte
substitution; no AST rewrite is required.

Edge case: when bold and italic share the same
delimiter character (e.g. both `asterisk`), the
boundary between `***bolditalic***` and `___both___`
becomes ambiguous to fix mechanically. Skip auto-fix
for triple-delimiter runs and emit the diagnostic
only.

### Error messages

```text
bold uses {actual}; configured style is {expected}
italic uses {actual}; configured style is {expected}
mixed emphasis delimiters: {outer} wraps {inner}
```

## Tasks

1. Scaffold `internal/rules/emphasisstyle/` with
   `rule.go`, `rule_test.go`, and `init()`
   `rule.Register`.
2. Implement `Check()` walking `*ast.Emphasis` and
   reading the source byte before each emphasis
   segment.
3. Implement `rule.Configurable` for `bold`,
   `italic`, and `forbid-mixed-nesting`.
4. Implement `rule.Defaultable` returning `false`.
5. Implement `Fix()` for bold and italic delimiter
   replacement; skip triple-delimiter runs.
6. Register as MDS040 in category `meta`.
7. Add fixture tests in
   `internal/rules/MDS040-emphasis-style/` covering
   each delimiter combination, mixed nesting, triple
   delimiters, and emphasis inside code spans
   (must not flag).
8. Add rule README.

## Acceptance Criteria

- [ ] `**bold**` with `bold: asterisk` emits no
      diagnostic.
- [ ] `__bold__` with `bold: asterisk` emits one
      diagnostic and fixes to `**bold**`.
- [ ] `*italic*` with `italic: underscore` emits one
      diagnostic and fixes to `_italic_`.
- [ ] `_*x*_` with `forbid-mixed-nesting: true` emits
      one diagnostic for the mixed nest.
- [ ] `***x***` triple-delimiter run emits a
      diagnostic but is not auto-fixed.
- [ ] Emphasis inside `` `code` `` and fenced code
      blocks emits no diagnostic.
- [ ] Rule is disabled by default.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
