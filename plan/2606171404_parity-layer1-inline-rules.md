---
id: 2606171404
title: "Parity parse-skip: migrate the Layer-1 inline rules"
status: "✅"
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
| MDS068 | link-style                         | link form             |
| MDS034 | markdown-flavor                    | flavor-specific spans |

## Tasks

For each rule (all done):

1. [x] Add the nil-AST path that drives `lint.InlineBlocks` for the
   blocks the rule cares about, reusing the existing inline extraction.
   MDS053 is cross-block: its reference def/use map is assembled by
   walking every block's re-parsed inline spans.
2. [x] Keep the diagnostic byte-identical: same line, column, message.
3. [x] Regenerate the walk audit and sync the embedded
   [rulelayer copy](../internal/rulelayer/rule_walk_audit.json).
4. [x] Add a `TestCheck_NilASTMatchesAST` unit test covering the inline
   edge cases — emphasis at a heading's end, a link inside emphasis,
   a code span holding bracket text.

## Result

All eleven rules now serve the parse-skipped (nil-AST) path from the
Layer-1 inline parse. Each is gated byte-identical to the AST. The gate
is a `TestCheck_NilASTMatchesAST` unit test plus the corpus
equivalence harness.

Eight resolve to the audit's `A-no-skipping` (Layer 0) category:
MDS005, MDS017, MDS042, MDS049, MDS053, MDS063, MDS068, and MDS034.
MDS053 assembles its reference def/use map across every run. MDS034
reads bare URLs from the inline runs via the new
`flavor.BareURLFindingsInTree`. It reads alert blockquotes from the
Layer 0 `BlockQuote` spans via the new `flavor.IsAlertMarkerLine` and
`AlertFinding`. The dual-parser features still detect from the body.

Three stay `B-prose-only`. MDS041 reads inline HTML and HTML-block
content. MDS050 scans code-block and HTML-block bodies under
`check-code` and `check-html`. MDS052 reads code-span content.

Each of the three is nil-AST-safe with a working inline path. The
audit's code-perturbation probe scrambles the very content these rules
read, so they are code-content-sensitive by design. MDS066 reached the
same terminal classification in plan 2606171402.

## Acceptance Criteria

- [x] Each rule resolves to Layer 0 / Layer 1: eight are `A-no-skipping`
      (Layer 0); MDS041, MDS050, MDS052 are `B-prose-only`
      (code-content-sensitive by design, like MDS066) yet nil-AST-safe
      on the Layer-1 inline path.
- [x] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with them on.
- [x] `go test ./...` passes (the pre-existing `internal/release` PGO /
      code-signing failures are environment-only and untouched here).
