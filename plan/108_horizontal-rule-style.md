---
id: 108
title: Horizontal rule style rule
status: "🔲"
summary: >-
  New rule MDS042 that pins one of `---`, `***`, or
  `___` for thematic breaks and requires blank lines
  on both sides so the rule cannot be confused with
  a setext heading underline. Closes one of the
  ambiguity cases in "Exhibit C" of the bgslabs.org
  rant.
model: ""
---
# Horizontal rule style rule

## Goal

Let users pin a single delimiter for thematic breaks.
CommonMark accepts `---`, `***`, and `___` (with any
length ≥ 3 and any internal spaces). One of the three
also collides with the setext heading underline:
`---` under text becomes an `<h2>` instead of a
horizontal rule. This rule pins one delimiter and
requires surrounding blank lines so the collision
cannot occur.

## Background

### What goldmark exposes

Thematic breaks are `*ast.ThematicBreak`. The node
carries no delimiter information; the rule must read
the source line that produced the node.

Setext headings (`Title\n=====` or `Title\n-----`)
are `*ast.Heading` with `IsSetext` true. MDS002
already lets users pin ATX-only, which removes the
collision risk from one direction. MDS042 closes the
other direction: even with setext allowed, a
horizontal rule must not look like a setext underline.

### Why a separate rule from MDS002

MDS002 is about *headings*. MDS042 is about
*thematic breaks*. They share an underlying syntax
collision but operate on disjoint AST node types and
have independent toggles.

## Design

### Configuration

```yaml
rules:
  horizontal-rule-style:
    style: dash          # dash | asterisk | underscore
    length: 3            # required exact length
    require-blank-lines: true
```

Category: `whitespace`. Disabled by default (opt-in).

### Detection

Walk `*ast.ThematicBreak`. For each node:

1. Read the source line. Strip leading/trailing
   whitespace.
2. If any character is not the configured delimiter,
   emit `wrong delimiter`.
3. If internal spaces exist, emit `delimiter has
   internal spaces`.
4. If the visible character count differs from
   `length`, emit `wrong length`.
5. If `require-blank-lines` is true and the previous
   or next line is non-blank, emit `missing blank
   line`.

### Auto-fix

Replace the line with the canonical delimiter
repeated `length` times. Insert a blank line above
and below if missing.

### Error messages

```text
horizontal rule uses {actual}; configured style is {expected}
horizontal rule has internal spaces
horizontal rule has length {actual}; configured length is {expected}
horizontal rule needs a blank line {above|below}
```

## Tasks

1. Scaffold `internal/rules/horizontalrulestyle/`.
2. Implement `Check()` walking `*ast.ThematicBreak`.
3. Implement `rule.Configurable` for `style`,
   `length`, and `require-blank-lines`.
4. Implement `Fix()` rewriting the line and adding
   surrounding blank lines.
5. Register as MDS042 in category `whitespace`.
6. Add fixture tests covering each delimiter
   choice, wrong length, internal spaces, missing
   blank lines, and a thematic break adjacent to a
   setext heading underline.
7. Add rule README.

## Acceptance Criteria

- [ ] `---` with `style: dash` and `length: 3` emits
      no diagnostic.
- [ ] `***` with `style: dash` emits one diagnostic
      and fixes to `---`.
- [ ] `- - -` emits an internal-spaces diagnostic.
- [ ] `-----` with `length: 3` emits a length
      diagnostic and fixes to `---`.
- [ ] A thematic break with no blank line above
      emits one diagnostic when
      `require-blank-lines: true`.
- [ ] A setext heading underline (`====` after
      text) emits no diagnostic from this rule.
- [ ] Rule is disabled by default.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
