---
id: 2606171403
title: "Parity parse-skip: migrate the Layer-0 list and blockquote rules"
status: "🔲"
summary: >-
  Add a nil-AST path to the parity rules that read list and blockquote
  structure — MDS014 blank-line-around-lists, MDS016 list-indent,
  MDS045 list-marker-style, MDS046 ordered-list-numbering, MDS061
  list-marker-space, MDS059 blockquote-whitespace — each gated
  byte-identical to the AST across the corpus.
model: opus
depends-on: [2606171258]
---
# Parity parse-skip: migrate the Layer-0 list and blockquote rules

## Goal

Move the list and blockquote parity rules onto the Layer-0 block scan,
so they no longer force the parse.

## Background

List and quote spans are the subtle case. CommonMark list parsing
folds in loose vs tight lists, nesting, and lazy continuation, so a
Layer-0 span boundary can drift from the AST. The
[Layer-0 scanner](../internal/lint/layer0.go) emits `BlockList`,
`BlockListItem`, and `BlockQuote` spans and descends into their bodies.

Each rule needs the marker shape (`-` / `*` / `+`, or `1.` / `1)`),
the marker indent, and the inter-item spacing — all line-level facts.
Verify the span boundaries match the AST before trusting them.

| Rule   | Name                    | Reads                     |
| ------ | ----------------------- | ------------------------- |
| MDS014 | blank-line-around-lists | top-level list span edges |
| MDS016 | list-indent             | item marker indent        |
| MDS045 | list-marker-style       | bullet marker character   |
| MDS046 | ordered-list-numbering  | ordered marker sequence   |
| MDS061 | list-marker-space       | spaces after the marker   |
| MDS059 | blockquote-whitespace   | `>` marker spacing        |

## Tasks

For each rule:

1. First confirm the relevant span boundary matches the AST across the
   corpus. Add the nil-AST path only once it does.
2. Add a `CheckBlock` (or span walk) reusing the existing extraction.
   Keep the diagnostic byte-identical: same line, column, message.
3. Regenerate the walk audit and sync the embedded
   [rulelayer copy](../internal/rulelayer/rule_walk_audit.json).
4. Add a `TestCheck_NilASTMatchesAST` unit test covering nested lists,
   a loose list, and a list that holds a code fence.

## Acceptance Criteria

- [ ] Each rule resolves to Layer 0 (audit `A-no-skipping`).
- [ ] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with them on.
- [ ] `go test ./...` passes.
