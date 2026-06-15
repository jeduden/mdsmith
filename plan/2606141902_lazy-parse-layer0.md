---
id: 2606141902
title: "Lazy parse: Layer 0 block scanner and parse-skip"
status: "✅"
summary: >-
  Build the single-pass block scanner (Layer 0) and
  re-back the block-level projections on it, so a config
  whose enabled rules all resolve to Layer 0 skips the
  goldmark parse entirely. Delivers the "simple configs
  run at line-scanner speed" win and the engine gate the
  later stages extend.
model: opus
depends-on: [2606141901]
---
# Lazy parse: Layer 0 block scanner and parse-skip

## Goal

Add a cheap block scanner and route the block-level
projections through it. When every enabled rule resolves
to Layer 0, the engine skips the goldmark parse. Simple
configs then run with no AST.

## Background

See the [lazy-parse research][research]. Rules read
projections, not the raw tree.
[CollectCodeBlockLines][cbl] alone backs 15 rules. Those
projections are the lazy seam.

Layer 0 is one forward pass over `f.Lines`. It records a
per-line class, the code-block and PI line sets, the
block spans, and the front-matter bounds. It allocates
no node tree.

The seam already exists in code. Re-back the projection
functions on Layer 0 with a Layer 2 fallback. Most rules
need no change at all.

[Plan 2606022126][audit] already sorted each rule by its
AST need. It used a nil-AST probe and a code-block
perturbation. Its manifest
(`internal/integration/testdata/rule_walk_audit.json`) says
which rules can reach Layer 0. Reuse it. Do not redo that
sort by hand. Its `ProseRanges` projection also exists.

## Tasks

1. Write the Layer 0 scanner: per-line class, code-block
   and PI line sets, block spans, front-matter bounds.
2. Re-back `CollectCodeBlockLines`, the PI line set, and
   the [astutil][astutil] section/heading/text helpers
   on Layer 0, keeping a Layer 2 fallback.
3. Record each rule's resolved layer (a small
   annotation beside the existing kind scope).
4. Add the engine gate. Skip
   [`NewFileFromSourcePooled`][newfile] when every
   enabled rule resolves to Layer 0. The scanner does not
   descend into a list item's content, so a fenced or
   indented code block inside a list item is the one
   shape where its `CodeBlockLines` can differ from the
   AST; the gate stays sound by also standing down for
   any source that may hold a code block
   (`lint.SourceMayHaveCodeBlock`), skipping the parse
   only for code-free files until the block-content work
   in plan 2606141903 lands.
5. Add an equivalence harness. Diff Layer 0 projections
   against the AST-derived ones across the corpus and
   the rule fixtures.

## Acceptance Criteria

- [x] A line-and-projection-only config skips the parse,
      proven by a test or profile.
- [x] `CollectCodeBlockLines` output is byte-identical
      between Layer 0 and the AST across the corpus.
- [x] All existing rule fixtures pass unchanged.
- [x] The Layer 0 equivalence gate is green.
- [x] All tests pass: `go test ./...`

[research]: ../docs/research/benchmarks/lazy-parse-architecture.md
[audit]: 2606022126_lines-only-rule-audit.md
[cbl]: ../internal/lint/codeblocks.go
[astutil]: ../internal/rules/astutil/astutil.go
[newfile]: ../internal/lint/filepool.go
