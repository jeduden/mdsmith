---
id: 2606141904
title: "Lazy parse: Layer 1 light inline index"
status: "🔲"
summary: >-
  Build the byte-level inline index (links, autolinks,
  images, code spans, raw HTML, reference defs and uses)
  and re-back the inline rules and LinkReferences on it.
  With Layer 0 and Layer 1 together, parity skips the
  full parse — the last step to beating gomarklint.
model: opus
depends-on: [2606141902]
---
# Lazy parse: Layer 1 light inline index

## Goal

Add a targeted inline scanner for the inline rules. It
finds links, images, code spans, raw HTML, and reference
definitions and uses. It does not run the emphasis
delimiter algorithm. Parity's inline rules then stop
forcing the full parse.

## Background

See the [lazy-parse research][research]. About a dozen
rules need inline detail. The link and reference rules
need links and the reference map. The code-span and
raw-HTML rules need those spans.

None need general emphasis. The general emphasis rule,
`emphasis-style`, stays on Layer 2. But one parity rule
resists: `no-emphasis-as-heading` reads a parsed emphasis
node. Its real need is bounded. Is a whole paragraph one
emphasis span? That check belongs on Layer 1. It is the
gate on parity shedding the parse (see the [research
note][research]).

Reference matching is the correctness risk. CommonMark
folds case and collapses whitespace in labels. The index
must normalize labels exactly as goldmark does. Lift its
normalization as a pure function so the two agree.

## Tasks

1. Write the inline index scanner: links, autolinks,
   images, code spans, raw HTML, and `[label]: url`
   definitions and uses.
2. Lift goldmark's reference-label normalization (case
   fold plus whitespace) into a shared pure function.
3. Re-back [`LinkReferences`][newfile] and the inline
   rules (`no-bare-urls`, `no-empty-alt-text`,
   `link-validity`, `no-space-in-code-spans`,
   `no-inline-html`, the reference rules) on the index.
4. Mark each inline rule's resolved layer and extend the
   parse-skip gate.
5. Add a bounded whole-paragraph-emphasis detector to the
   index. Re-back `no-emphasis-as-heading` on it. Gate it
   byte-identical to goldmark's lone-emphasis-child result.
6. Fallback if an edge case defeats the detector: make
   Layer 2 per-block. `no-emphasis-as-heading` parses
   inlines for only the paragraphs whose first non-space
   byte is `*` or `_`. The gate then keys on "no
   whole-document parse", not "no Layer 2".

## Acceptance Criteria

- [ ] With Layer 0 and Layer 1, the full parity config
      runs with no whole-document goldmark parse.
- [ ] `no-emphasis-as-heading` output is byte-identical to
      its AST path across the corpus and fixtures.
- [ ] Reference matching is byte-identical to the AST
      path, including case folding, across the corpus.
- [ ] All existing rule fixtures pass unchanged.
- [ ] `mdsmith check -c parity` beats gomarklint on
      benchmark 2.
- [ ] All tests pass: `go test ./...`

[research]: ../docs/research/benchmarks/lazy-parse-architecture.md
[newfile]: ../internal/lint/file.go
