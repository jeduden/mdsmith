---
id: 124
title: No space inside code spans rule
status: "✅"
summary: >-
  New rule MDS048 that flags inline code spans with
  leading or trailing whitespace inside the backticks
  (`` ` x ` `` instead of `` `x` ``). Closes the gap
  with markdownlint MD038.
model: sonnet
---
# No space inside code spans rule

## Goal

Let users forbid stray whitespace inside inline code
spans. CommonMark strips one optional space on each
side of a code span when both sides have one, but any
other leading or trailing whitespace renders verbatim
and is almost always a typo (`` `  x` ``,
`` `x ` ``, `` `x  ` ``). markdownlint covers this as
[MD038][md038]; mdsmith does not.

## Background

### What goldmark exposes

Inline code spans are `*ast.CodeSpan`. The node's
text segment range covers the bytes between the
backtick delimiters, *before* CommonMark's "trim one
space on each side if both sides have one" rule is
applied. The rule must read those raw bytes to
distinguish "balanced single space" (legal) from any
other whitespace pattern (flagged).

### Why a separate rule

MDS010 pins fence style and MDS011 requires a fence
language. Neither inspects the *contents* of inline
code spans. A dedicated rule keeps the toggle
independent of fenced-block policy.

## Design

### Configuration

```yaml
rules:
  no-space-in-code-spans: true
```

Category: `whitespace`. Disabled by default (opt-in).
No tunables in v1 — the only choice is whether to
enforce.

### Detection

Walk `*ast.CodeSpan`. For each node:

1. Read the span's source bytes (between the
   delimiters).
2. If both the first and last byte are a single
   ASCII space and no other whitespace is present at
   the boundaries, treat as the CommonMark
   single-space-trim case and skip.
3. Otherwise, if the bytes start with whitespace,
   emit `code span has leading whitespace`.
4. If the bytes end with whitespace, emit `code
   span has trailing whitespace`.

Tabs, newlines, and runs of two or more spaces at
either boundary always emit. The single-space-trim
exception only covers one ASCII space on each side.

### Auto-fix

Trim leading and trailing whitespace from the span
bytes. Preserve the delimiter count (one or more
backticks). When the trimmed body becomes empty, do
not auto-fix — emit only the diagnostic.

### Error messages

```text
code span has leading whitespace
code span has trailing whitespace
```

## Tasks

1. [x] Scaffold `internal/rules/nospaceincodespans/`
   with `rule.go`, `rule_test.go`, and the `init()`
   `rule.Register` call.
2. [x] Implement `Check()` walking `*ast.CodeSpan`
   and reading the raw source bytes between
   delimiters.
3. [x] Implement `Fix()` that trims whitespace inside
   the delimiters while preserving backtick count.
4. [x] Implement `rule.Defaultable` returning `false`.
5. [x] Register as MDS049 in category `whitespace`.
   (MDS048 was already taken by git-hook-sync.)
6. [x] Add fixture tests in
   `internal/rules/MDS049-no-space-in-code-spans/`
   covering: balanced single space (legal), leading
   space, trailing space, both-side double space,
   tab, and the empty-after-trim edge case.
7. [x] Add rule README following the MDS012 template.

## Acceptance Criteria

- [x] `` `x` `` emits no diagnostic.
- [x] `` ` x ` `` (balanced single space) emits no
      diagnostic.
- [x] `` ` x` `` emits one leading-whitespace
      diagnostic and fixes to `` `x` ``.
- [x] `` `x ` `` emits one trailing-whitespace
      diagnostic and fixes to `` `x` ``.
- [x] `` `  x  ` `` (double space each side) emits
      both diagnostics and fixes to `` `x` ``.
- [x] `` `\tx` `` (leading tab) emits a leading-
      whitespace diagnostic.
- [x] An empty-after-trim span (e.g. `` `   ` ``)
      emits diagnostics but is not auto-fixed.
- [x] Rule is disabled by default.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] `mdsmith check .` passes on the repo with the
      rule disabled (no regression for existing
      docs).
