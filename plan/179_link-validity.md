---
id: 179
title: Reversed and empty link rule
status: "🔲"
model: opus
depends-on: []
summary: >-
  New rule MDS062 (provisional) covering markdownlint
  MD011 (reversed link syntax) and MD042 (empty link).
  Both are correctness defects — the link does not work —
  so the rule is default-enabled. MD011 is autofixable.
---
# Reversed and empty link rule

## Goal

Catch links that silently do not work: reversed
`(text)[url]` syntax and links with an empty target or
empty text. This closes the MD011 / MD042 gap from the
[linter comparison](../docs/background/markdown-linters.md).

## Background

- `(label)[ref]` — MD011 no-reversed-links. CommonMark
  renders this as literal text, so the author's intended
  link is silently lost.
- `[text]()` or `[]( )` or `[](url)` with empty text —
  MD042 no-empty-links.

goldmark parses a real link as `*ast.Link`, so an empty
target shows up there. The reversed form is *not* a link
node — it stays `*ast.Text`, so MD011 needs a small regex
over text runs (`\(([^)]+)\)\[([^\]]+)\]`) that skips code
spans and autolinks.

## Design

- Rule ID: MDS062 (provisional), category `correctness`,
  default-enabled.
- MD042: walk `*ast.Link` / `*ast.Image`; flag an empty or
  whitespace-only destination, or empty visible text.
  Fragment-only (`#x`) and `<>` empty-destination per
  CommonMark are flagged. No autofix (no safe target).
- MD011: scan text segments for the reversed pattern;
  flag it and autofix to `[text](url)`. Skip matches
  inside code spans, code blocks, and directive bodies.

## Tasks

1. Scaffold `internal/rules/linkvalidity/`.
2. Implement the MD042 AST check.
3. Implement the MD011 text-scan check with the
   skip-context list and its autofix.
4. Fixture tests under the provisional
   `internal/rules/MDS062-*` directory: reversed link,
   empty target, empty text, code-span false positive,
   autolink, directive.
5. Rule README; regenerate the docs catalog and index.
6. Add the MD011 / MD042 rows to the
   [linter comparison](../docs/background/markdown-linters.md).

## Acceptance Criteria

- [ ] `(text)[url]` is flagged and fixed to
      `[text](url)`.
- [ ] `[text]()` and `[](url)` are flagged with no
      autofix.
- [ ] A reversed pattern inside a code span or fenced
      block is not flagged.
- [ ] A normal `[text](url)` and a `<https://x>`
      autolink are clean.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes
