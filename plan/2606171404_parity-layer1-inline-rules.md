---
id: 2606171404
title: "Parity parse-skip: migrate the Layer-1 inline rules"
status: "🔲"
summary: >-
  Add a nil-AST path to the parity rules that read inline content —
  heading text, links, emphasis, code spans, inline HTML, reference
  definitions — by driving the shared per-block inline re-parse
  (lint.InlineBlocks) instead of the full document parse, each gated
  byte-identical to the AST across the corpus.
model: opus
depends-on: [2606171258]
---
# Parity parse-skip: migrate the Layer-1 inline rules

## Goal

Move the inline-content parity rules onto the Layer 1 path, so they
read re-parsed inline spans instead of forcing the document parse.

## Background

These rules read flattened inline content, not just lines: a heading's
text, a link's text and destination, an emphasis run, a code span, an
inline HTML tag, or the reference-definition map. The line scan cannot
produce that. The Layer 1 index
([lint.InlineBlocks](../internal/lint/inline_blocks.go)) re-parses one
block's inline tree on demand, so a rule reaches inline spans without
the whole-document parse.

| Rule   | Name                               | Reads                 |
| ------ | ---------------------------------- | --------------------- |
| MDS005 | no-duplicate-headings              | heading text          |
| MDS017 | no-trailing-punctuation-in-heading | heading text          |
| MDS041 | no-inline-html                     | inline HTML nodes     |
| MDS042 | emphasis-style                     | emphasis markers      |
| MDS049 | no-space-in-link-text              | link text edges       |
| MDS050 | proper-names                       | inline text runs      |
| MDS052 | no-space-in-code-spans             | code-span content     |
| MDS053 | no-unused-link-definitions         | ref defs and uses     |
| MDS063 | descriptive-link-text              | link text             |
| MDS067 | callout-type                       | blockquote callout    |
| MDS068 | link-style                         | link form             |
| MDS034 | markdown-flavor                    | flavor-specific spans |

## Tasks

For each rule:

1. Add the nil-AST path that drives `lint.InlineBlocks` for the blocks
   the rule cares about, reusing the existing inline extraction.
2. Keep the diagnostic byte-identical: same line, column, message.
3. Regenerate the walk audit and sync the embedded
   [rulelayer copy](../internal/rulelayer/rule_walk_audit.json).
4. Add a `TestCheck_NilASTMatchesAST` unit test covering the inline
   edge cases — emphasis at a heading's end, a link inside emphasis,
   a code span holding bracket text.

## Acceptance Criteria

- [ ] Each rule resolves to Layer 0 / Layer 1 (audit `A-no-skipping`).
- [ ] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with them on.
- [ ] `go test ./...` passes.
