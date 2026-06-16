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

## Implementation Status

This plan landed a first slice. The inline index now scans
**code spans**. It re-backs the two rules whose only AST
need was the code-span projection. The output stays
byte-identical to the AST path.

Done:

- [x] Byte-level inline scanner for code spans
      ([inline_index.go][newfile-idx]). It skips lines
      inside fenced/indented code blocks, HTML blocks,
      and PI blocks via the Layer 0 scan, reproduces
      CommonMark's single-space content trim, and derives
      the literal range the same two-step way goldmark
      does (content bounds extended over adjacent
      backticks).
- [x] [`CodeSpanContentRanges` / `CodeSpanLiteralRanges`][newfile]
      re-backed on the index for the nil-AST path.
- [x] Reference-label normalization: goldmark already
      exports the pure `util.ToLinkReference` (case fold +
      whitespace collapse); MDS054 already calls it, so no
      lift was needed.
- [x] MDS047 (ambiguous-emphasis) and MDS054
      (no-undefined-reference-labels) promoted from the
      `astProjectionConsumers` AST override to Layer 0 in
      [`internal/rulelayer`][rulelayer]; the parse-skip
      gate now admits them.
- [x] Corpus equivalence guards: a Layer 1 code-span
      equivalence test over every parse-skip-eligible
      corpus file, and the existing Layer 0 corpus gate
      equivalence test now green with MDS047/MDS054
      enabled.

This branch re-backs the hybrid inline rules via a
**shared per-block lazy parse**, not a bespoke
link/image/autolink/raw-HTML byte scanner.
`lint.InlineBlocks` re-parses each contiguous run of
inline-bearing lines with goldmark's own parser, once per
file and cached. The inline tree is byte-identical by
construction. Runs (not single-line Layer 0 spans) keep a
construct that wraps onto a list or quote continuation line
whole. The document's link reference definitions are seeded
into each run's context, so a cross-block `[text][ref]`
still resolves.

Each rule maps the run-local segment offsets back to the
document with the run's start offset via
`lint.WalkInlineNodes`. The NodeChecker rules
(MDS012/018/032) implement `rule.InlineChecker`. That
marker routes a NodeChecker to its own `Check` on a nil-AST
File. `link-validity` (MDS062) is a plain `Check` rule, so
the engine already calls its `Check` on that path.

Done (this branch):

- [x] `no-emphasis-as-heading` (MDS018): the
      whole-paragraph-emphasis detector
      ([inline_emphasis.go][newfile-emph]) reads the shared
      run parse and flags every paragraph (including those
      goldmark nests in a block quote) whose sole inline
      child is one emphasis span, byte-identical to
      goldmark's lone-emphasis-child result.
- [x] `no-bare-urls` (MDS012): the same Text-node URL check
      runs over the shared run parse; URLs inside links,
      autolinks, and code spans are skipped exactly as on
      the AST path.
- [x] `no-empty-alt-text` (MDS032): the per-Image empty-alt
      check runs over the shared run parse with offset
      remapping.
- [x] `link-validity` (MDS062): the empty-link check runs
      over the shared run parse; the reversed-link check was
      already byte-level on the re-backed code-span ranges.
- [x] All four reclassified `A-no-skipping` by the audit;
      `internal/rulelayer` admits them to Layer 0. Corpus
      and end-to-end gate equivalence
      ([inline_rule_equivalence_test.go][newfile-eq])
      pin every re-backed rule byte-identical to the AST
      path on the gate-eligible corpus.

Still deferred:

- [ ] `no-inline-html` (MDS041) and `no-space-in-code-spans`
      (MDS052): both opt-in (not in the parity set), so they
      do not block the parity gate. MDS052 needs the
      code-span literal bounds it already reads via the
      re-backed projection but still walks via `WalkNodes`;
      MDS041 needs the raw-HTML span projection. Left for a
      follow-up.
- [ ] `LinkReferences` re-backed on a byte-level
      reference-definition scanner. It still parses the
      whole document once, lazily, the first time a
      reference rule reads it on a nil-AST file containing
      `[`. The byte-identity risk (multi-line labels,
      escaped destinations, titles) matches the link
      scanner's; the reference rules are already correct and
      Layer 0 through the lazy parse, so this is the
      remaining piece to shed that one residual parse.

## Acceptance Criteria

- [~] With Layer 0 and Layer 1, the full parity config
      runs with no whole-document goldmark parse. Every
      inline parity rule (MDS012/018/032/062, MDS047,
      MDS054) is now Layer 0. Residual full-document parses
      remain only for (a) `LinkReferences` on files
      containing `[` (lazy, on-demand — see status) and (b)
      the ~20 block-level parity rules (MDS001–011, 014–017,
      031, …) a sibling Layer 0 block plan owns, not this
      one. The inline rules no longer force a parse.
- [x] `no-emphasis-as-heading` output is byte-identical to
      its AST path across the corpus and fixtures
      (`TestInlineRuleEquivalence_Corpus`,
      `TestWholeParagraphEmphasis_Equivalence`).
- [x] Reference matching is byte-identical to the AST
      path, including case folding, across the corpus.
      MDS054's labels normalize through goldmark's own
      `util.ToLinkReference`, and the Layer 0 corpus gate
      equivalence test passes with it enabled.
- [x] All existing rule fixtures pass unchanged.
- [ ] `mdsmith check -c parity` beats gomarklint on
      benchmark 2. (Not measured here; the parity rule set
      still forces a parse via the deferred hybrid rules,
      so the benchmark gain awaits the follow-up.)
- [x] All tests pass: `go test ./...` (excluding the
      pre-existing `internal/release` PGO tests, which fail
      on commit-signing infra unrelated to this change).

[newfile-idx]: ../internal/lint/inline_index.go
[newfile-emph]: ../internal/lint/inline_emphasis.go
[newfile-eq]: ../internal/integration/inline_rule_equivalence_test.go
[rulelayer]: ../internal/rulelayer/rulelayer.go

[research]: ../docs/research/benchmarks/lazy-parse-architecture.md
[newfile]: ../internal/lint/file.go
