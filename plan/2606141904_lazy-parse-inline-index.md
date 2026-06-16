---
id: 2606141904
title: "Lazy parse: Layer 1 light inline index"
status: "🔳"
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
- [x] Reference matching is byte-identical to the AST
      path, including case folding, across the corpus.
- [ ] All existing rule fixtures pass unchanged.
- [ ] `mdsmith check -c parity` beats gomarklint on
      benchmark 2.
- [x] All tests pass: `go test ./...`

## Implementation Status

PR #632 (separate branch) added the inline code-span
index and the inline block grouper. It re-backed four
inline rules (MDS012/018/032/062) and two node-checker
rules (MDS047/054) on the lazy parse. It deferred one
piece. `LinkReferences` still forced a full lazy parse on
the nil-AST path.

This branch sheds that residual parse. The reference
criterion is now done:

- [x] Reference matching is byte-identical to the AST path,
      including case folding, across the corpus.

What landed here:

- `parser.ScanReferenceDefinitions` (new,
  `pkg/goldmark/parser/link_ref_scan.go`): drives goldmark's
  own `parseLinkReferenceDefinition` over a supplied set of
  line segments and records each definition via
  `AddReference`. Reusing goldmark's exact definition parser
  — including the multi-line label, angle/bare destination,
  `"`/`'`/`(` title, title-on-next-line, and `util.ToLinkReference`
  normalization paths — guarantees byte-identity instead of
  re-deriving the §4.7 grammar by hand.
- `scanLinkReferences` (new, `internal/lint/linkrefscan.go`):
  walks `Layer0` block spans, and for each top-level
  paragraph whose head holds a `]:` after a leading `[`,
  feeds that paragraph's line segments to the parser scanner.
  First-wins dedup falls out of `AddReference`, so feeding
  paragraphs in document order reproduces the full parse.
- `scanNeedsFallback` gates correctness: a `]:` nested in a
  block quote or list block (which the paragraph-head scanner
  does not descend into) triggers a single lazy full parse,
  so those rare shapes stay byte-identical.
- [`LinkReferences`][newfile] now picks, in order: the
  captured parse context (NewFile path), the byte-level
  scanner (nil-AST path, no fallback needed), or a lazy full
  parse (fallback). The nil-AST path no longer parses the
  whole document for the common case.
- Tests: parser-level equivalence
  (`link_ref_scan_test.go`), lint-level case + container-
  fallback + nil-AST tests, and a corpus equivalence test
  that compares the scanner against the full parse for every
  repo `.md` file (1043 files, 0 divergences).

The other plan criteria stay out of scope for this branch.
The full Layer 1 inline index, `no-emphasis-as-heading` on
Layer 1, and the parity-beats-gomarklint benchmark remain
tracked by the broader plan and by PR #632.

[research]: ../docs/research/benchmarks/lazy-parse-architecture.md
[newfile]: ../internal/lint/file.go
