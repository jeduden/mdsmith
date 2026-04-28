---
id: 111
title: Ambiguous emphasis rule
status: "✅"
summary: >-
  New rule MDS047 that flags emphasis runs whose
  meaning a human cannot predict at a glance even
  though CommonMark resolves them deterministically.
  Targets the parser-stress cases ("Exhibit A") and
  the ReDoS pattern shape called out in the
  bgslabs.org rant.
model: sonnet
---
# Ambiguous emphasis rule

## Goal

Let users forbid emphasis sequences that are
*technically* valid but unreadable. The CommonMark
emphasis algorithm runs a left-flanking and
right-flanking delimiter scan that can pair `*` and
`_` runs in non-obvious ways. The output is
well-defined; the source is not. Examples from the
rant include `*****\*a*` and `***Peter* Piper**
_Picked___a___Pack_`. The rule names a small,
auditable set of "you cannot tell at a glance"
shapes and refuses them.

## Background

### What counts as ambiguous

Three patterns cover most of the surprise:

1. **Long delimiter runs** — three or more contiguous
   `*` or `_` characters at one boundary (e.g.
   `***`, `____`, `*****`). CommonMark splits the
   run between bold and italic by counting from the
   matching closing run; the split rarely matches
   author intent.
2. **Adjacent same-character delimiters across word
   boundaries** — `__a__b__` or `*a*b*`. The
   flanking rules can pair these multiple ways
   depending on surrounding whitespace. Different
   parsers have historically disagreed on these.
3. **Escaped emphasis adjacent to delimiter runs** —
   `*\*x*`, `*****\*a*`. Backslash-escapes inside a
   long run interact with the flanking scan in ways
   no human reads correctly. This is also the shape
   the markdown-it ReDoS CVE exploited.

Each pattern is a static check on the source bytes.
The AST is not consulted because goldmark has
already collapsed the ambiguity.

### Why source-only

Goldmark resolves these patterns to a definite AST.
Walking the AST tells the rule what the parser
chose, not whether a human could have predicted
that choice. The rule scans the raw source line for
the suspicious shapes instead.

### Interaction with MDS042

MDS042 (emphasis-style) pins which delimiter is
used. MDS047 catches the ambiguous *combinations*
that survive a delimiter pin. They can fire
independently on the same line.

## Design

### Configuration

```yaml
rules:
  ambiguous-emphasis:
    max-run: 2          # delimiter run length cap
    forbid-escaped-in-run: true
    forbid-adjacent-same-delim: true
```

Category: `meta`. Disabled by default (opt-in).

### Flavor activation

[Plan 112](112_flavor-profiles.md) ships profiles
that auto-enable this rule:

- `profile: portable` activates with
  `max-run: 2`, `forbid-escaped-in-run: true`, and
  `forbid-adjacent-same-delim: true`.
- `profile: github` does not activate
  this rule.
- `profile: plain` activates with the same
  settings as `portable`.

User overrides on top of the profile still win via
deep-merge.

### Detection

Read the source line by line, skipping ranges
covered by `*ast.CodeSpan`, `*ast.FencedCodeBlock`,
and `*ast.CodeBlock`.

For each non-code range:

1. Find every contiguous run of `*` or `_`. If the
   run length exceeds `max-run`, emit one
   diagnostic.
2. If the run contains a backslash-escaped `*` or
   `_` adjacent to it (`*\*` or `_\_`) and
   `forbid-escaped-in-run` is true, emit one
   diagnostic.
3. Find `<delim>word<delim>word<delim>` patterns
   where the same single-character delimiter appears
   three times on the line with non-whitespace
   between them. When `forbid-adjacent-same-delim`
   is true, emit one diagnostic.

### Auto-fix

No auto-fix. The right rewrite depends on author
intent, which the rule cannot recover. The
diagnostic message suggests adding a space, an HTML
entity, or splitting the run.

### Error messages

```text
emphasis run of {n} delimiters; max is {max-run}
escaped delimiter inside emphasis run
adjacent same-delimiter emphasis is ambiguous
```

## Tasks

1. [x] Scaffold `internal/rules/ambiguousemphasis/`.
2. [x] Implement source-range computation that
   excludes code spans and code blocks.
3. [x] Implement the three pattern detectors.
4. [x] Implement `rule.Configurable` for `max-run`,
   `forbid-escaped-in-run`, and
   `forbid-adjacent-same-delim`.
5. [x] Register as MDS047 in category `meta`.
6. [x] Add fixture tests covering each pattern,
   patterns inside code spans (must not flag),
   patterns inside fenced code blocks (must not
   flag), and the rant's exact strings
   (`*****\*a*`, `***Peter* Piper**`).
7. [x] Add rule README.

## Acceptance Criteria

- [x] `**bold**` emits no diagnostic.
- [x] `***bold-italic***` emits one diagnostic when
      `max-run: 2`. The two symmetric `***` runs
      collapse into a single report by deduplicating
      diagnostics per `(char, length)` per line.
- [x] `*****\*a*` emits diagnostics for both run
      length and escaped-in-run.
- [x] `__a__b__` emits one
      adjacent-same-delim diagnostic.
- [x] The same patterns inside `` `code` `` or a
      fenced block emit no diagnostic.
- [x] No auto-fix is attempted.
- [x] Rule is disabled by default. `DefaultSettings`
      ships `max-run: 0` and both bool flags `false`
      so the rule remains a no-op in any pipeline
      that wires it without explicit settings;
      profile activation in plan 112 supplies the
      active values.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
