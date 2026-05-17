---
id: 177
title: Blockquote whitespace rule
status: "🔲"
model: sonnet
depends-on: []
summary: >-
  New rule MDS060 (provisional) covering markdownlint
  MD027 (multiple spaces after the blockquote marker) and
  MD028 (blank line between two blockquotes). Autofix
  collapses the spaces; MD028 is flag-only.
---
# Blockquote whitespace rule

## Goal

Flag two blockquote defects: more than one space after
`>` and a blank line between two adjacent blockquotes that
silently joins or splits them depending on the renderer.
This closes the MD027 / MD028 gap from the
[linter comparison](../docs/background/markdown-linters.md).

## Background

- `>  text` — MD027 no-multiple-space-blockquote.
- A `>` block, a blank line, then another `>` block —
  MD028 no-blanks-blockquote. Renderers disagree on
  whether this is one quote or two.

goldmark exposes `*ast.Blockquote`. MD027 is a per-line
check on the marker run; MD028 inspects the gap between
two sibling blockquote nodes.

## Design

- Rule ID: MDS060 (provisional), category `style`,
  default-enabled.
- MD027: for each blockquote line, flag a marker followed
  by two or more spaces; autofix collapses to one space.
- MD028: when two `*ast.Blockquote` siblings are separated
  only by blank lines, flag the gap. No autofix — the
  intent (one quote vs two) is ambiguous, so a wrong
  rewrite is worse than a diagnostic.
- Skip nested code and directive bodies.

## Tasks

1. Scaffold `internal/rules/blockquotewhitespace/`.
2. Implement the MD027 per-line check and its autofix.
3. Implement the MD028 sibling-gap check (flag only).
4. Fixture tests under the provisional
   `internal/rules/MDS060-*` directory.
5. Rule README; regenerate the docs catalog and index.
6. Add the MD027 / MD028 rows to the
   [linter comparison](../docs/background/markdown-linters.md).

## Acceptance Criteria

- [ ] `>  quoted` is flagged and fixed to `> quoted`.
- [ ] `> a` / blank / `> b` emits one MD028-style
      diagnostic and is not auto-rewritten.
- [ ] A single blockquote with internal blank lines
      inside one `>` run is not flagged.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes
