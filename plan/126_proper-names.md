---
id: 126
title: Proper-name capitalization rule
status: "🔲"
summary: >-
  New rule MDS050 that pins capitalization for a
  user-defined list of proper names (e.g. `JavaScript`,
  `GitHub`, `mdsmith`) so casing stays consistent
  across the corpus. Closes the gap with markdownlint
  MD044.
model: sonnet
---
# Proper-name capitalization rule

## Goal

Let users pin the canonical spelling and casing of
proper names that recur across their docs.
`Javascript` vs `JavaScript`, `Github` vs `GitHub`,
`Markdown` vs `markdown` — these drift over time
without an automated check. markdownlint covers this
as [MD044][md044]; mdsmith does not.

## Background

### Prior art

markdownlint's MD044 takes a list of names plus two
toggles: `code_blocks` (also check inside code
blocks and spans) and `html_elements` (also check
inside raw HTML). Each name is matched whole-word and
case-insensitive. Any occurrence whose casing does
not match the configured spelling is reported.

### What goldmark exposes

Visible prose lives in `*ast.Text` nodes. Inline
code is `*ast.CodeSpan`. Code blocks are
`*ast.FencedCodeBlock` and `*ast.CodeBlock`. Link
text is the children of `*ast.Link`. Headings hold
text in `*ast.Text` children. The rule walks these
node types and runs a whole-word match per
configured name.

URLs (`*ast.AutoLink`, the URL inside `*ast.Link`)
are excluded — `github.com` is not a casing error.

### Whole-word matching

A "word" boundary is any non-ASCII-letter,
non-ASCII-digit, non-underscore character. `JavaScripts`
matches the configured `JavaScript`; `GitHubber` does
not match `GitHub` (ends in a letter, so the
preceding match fails the right boundary). Hyphens
and dots count as boundaries.

### Why a separate rule

MDS017 (whitespace-style) and MDS001 (line-length)
are typographic. Casing is semantic — it depends on
the project's vocabulary. A dedicated rule keeps the
project vocabulary out of the typography rules.

## Design

### Configuration

```yaml
rules:
  proper-names:
    names:
      - JavaScript
      - TypeScript
      - GitHub
      - mdsmith
    check-code: false
    check-html: false
```

Category: `prose`. Disabled by default (opt-in).
`names` is a list-typed setting and **appends** by
default — most teams want to extend an inherited
vocabulary, not replace it. Document this choice in
the `ApplySettings` handler (the same convention
used by `placeholders:`).

### Detection

For each visible-prose node (text in paragraphs,
headings, list items, link text, blockquotes):

1. For each configured name, scan the text bytes
   for case-insensitive matches that are bounded by
   word breaks.
2. Compare the matched bytes against the configured
   spelling byte-for-byte.
3. On mismatch, emit `proper name "{actual}" should
   be "{configured}"`.

When `check-code: true`, also walk `*ast.CodeSpan`,
`*ast.FencedCodeBlock`, and `*ast.CodeBlock`
contents. When `check-html: true`, also walk
`*ast.RawHTML` and `*ast.HTMLBlock` text.

### Auto-fix

Replace the matched bytes in `f.Source` with the
configured spelling. Whole-word boundaries make this
a safe in-place rewrite.

### Error messages

```text
proper name "{actual}" should be "{configured}"
```

## Tasks

1. Scaffold `internal/rules/propernames/` with
   `rule.go`, `rule_test.go`, and the `init()`
   `rule.Register` call.
2. Implement a whole-word matcher with a small
   helper covering ASCII letter/digit/underscore as
   the word class.
3. Implement `Check()` walking visible-prose nodes,
   then optionally code/HTML nodes per setting.
4. Implement `rule.Configurable` for `names`,
   `check-code`, and `check-html`. Implement
   `rule.ListMerger.SettingMergeMode("names")`
   returning `rule.MergeAppend`.
5. Implement `Fix()` rewriting the matched bytes.
6. Implement `rule.Defaultable` returning `false`.
7. Register as MDS050 in category `prose`.
8. Add fixture tests in
   `internal/rules/MDS050-proper-names/` covering:
   correct casing (clean), wrong casing in prose,
   wrong casing in heading, wrong casing in link
   text, wrong casing in code (skipped by default),
   wrong casing in code (flagged with
   `check-code: true`), word-boundary edge cases
   (`JavaScripts`, `GitHubber`), and append merge
   behavior across kind layers.
9. Add rule README following the MDS012 template.

## Acceptance Criteria

- [ ] `JavaScript is fun` with `names: [JavaScript]`
      emits no diagnostic.
- [ ] `Javascript is fun` emits one diagnostic and
      fixes to `JavaScript is fun`.
- [ ] `JAVASCRIPT` emits one diagnostic.
- [ ] `JavaScripts` (trailing letter) emits one
      diagnostic on the `JavaScript` portion.
- [ ] `GitHubber` does not match `GitHub` (no
      diagnostic).
- [ ] Heading `# Github` emits one diagnostic.
- [ ] `[Github](https://github.com)` emits one
      diagnostic on the link text only — the URL is
      not checked.
- [ ] An inline code span `` `javascript` `` emits
      no diagnostic when `check-code: false`.
- [ ] A fenced code block containing `javascript`
      emits a diagnostic only when `check-code: true`.
- [ ] `names:` set in a kind layer and again in an
      override appends — both names are checked.
- [ ] Rule is disabled by default.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes on the repo with the
      rule disabled (no regression for existing
      docs).
