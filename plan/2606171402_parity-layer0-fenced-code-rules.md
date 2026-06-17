---
id: 2606171402
title: "Parity parse-skip: migrate the Layer-0 fenced-code rules"
status: "🔳"
summary: >-
  Add a nil-AST path to the parity rules that read only a fenced code
  block's fence lines — MDS010 fenced-code-style, MDS011
  fenced-code-language, MDS031 unclosed-code-block, MDS065
  code-block-style, MDS066 commands-show-output — so each resolves to
  Layer 0, gated byte-identical to the AST across the corpus.
model: sonnet
depends-on: [2606171258]
---
# Parity parse-skip: migrate the Layer-0 fenced-code rules

## Goal

Move the fenced-code parity rules onto the Layer-0 block scan, so they
no longer force the parse.

## Background

The [Layer-0 scanner](../internal/lint/layer0.go) emits a
`BlockFencedCode` span per fenced block, fence lines included. MDS015
blank-line-around-fenced-code already reads those spans on the nil-AST
path. The code-skip unblock
(PR #644) confirmed the scanner matches the AST on code-bearing files,
fences inside list items included.

These rules read only the fence lines — the fence character, its run
length, and the info string — and the block body lines. None needs the
inline tree.

| Rule   | Name                 | Reads                         |
| ------ | -------------------- | ----------------------------- |
| MDS010 | fenced-code-style    | fence char (backtick / tilde) |
| MDS011 | fenced-code-language | info string presence          |
| MDS031 | unclosed-code-block  | an unterminated opening fence |
| MDS065 | code-block-style     | fenced vs indented form       |
| MDS066 | commands-show-output | shell info string + body      |

## Tasks

For each rule:

1. Add the nil-AST path — a `CheckBlock` over `BlockFencedCode` (and
   `BlockIndentedCode` where the rule needs it), reusing the existing
   fence parsing.
2. Keep the diagnostic byte-identical: same line, column, message.
3. Regenerate the walk audit and sync the embedded
   [rulelayer copy](../internal/rulelayer/rule_walk_audit.json).
4. Add a `TestCheck_NilASTMatchesAST` unit test with code-bearing
   inputs, including a fence inside a list item.

## Result so far

- [x] MDS010 fenced-code-style: `CheckBlock` reads the fence character
      from the `BlockFencedCode` span's opening line; `A-no-skipping`,
      corpus gate green.
- [x] MDS011 fenced-code-language: `CheckBlock` reads the info string
      from the opening line; `A-no-skipping`, corpus gate green.
- [x] MDS031 unclosed-code-block: added a `Closed` bool to the fenced
      `BlockSpan`, set from the `closed` flag `tryFence` already computes;
      `CheckBlock` flags `!span.Closed`. `A-no-skipping`, corpus gate
      green, with a 10-case unit test for the closure edges the corpus
      barely exercises (lone fence, info-no-content, trailing blank).
- [ ] MDS065 code-block-style, MDS066 commands-show-output.

## Acceptance Criteria

- [ ] Each rule resolves to Layer 0 (audit `A-no-skipping`).
- [ ] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with them on.
- [ ] `go test ./...` passes.
