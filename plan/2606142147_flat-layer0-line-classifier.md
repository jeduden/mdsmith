---
id: 2606142147
title: "Prototype: flat Layer-0 line classifier for line-length vs gomarklint"
status: "✅"
summary: >-
  Build a flat, node-tree-free line classifier (fence /
  heading / table / blank tracking over f.Lines), re-back
  line-length on it behind a default-off flag, skip the
  goldmark parse when line-length is the only enabled
  rule, and measure the pure-lint head-to-head against
  gomarklint on benchmark 2. Settles whether a true flat
  Layer 0 — not the block-only goldmark proxy — closes the
  ~1.26x residual the per-rule spike found on MDS001.
model: opus
depends-on: [2606141901]
---
# Prototype: flat Layer-0 line classifier for line-length vs gomarklint

## Goal

Can mdsmith's `line-length` rule reach gomarklint on the
neutral corpus? The [spike][spike] left a ~1.26x residual
on this rule, even with the inline parse gone and no
output. That residual is the goldmark block parse. This
prototype replaces it with a flat Layer-0 line classifier
and re-runs the head-to-head, to settle the number.

## Background

See the [per-rule bottleneck][bottleneck] in the
lazy-parse research note. The spike's block-only flag is
an upper bound on Layer 0: it suppresses the inline phase
but still runs goldmark's block parse, which builds a
block *node tree* (~14 ms serial — the dominant lint cost
for MDS001). gomarklint never builds a tree; it tracks
fences and headings with byte comparisons over its line
slice.

The per-rule decomposition isolated three costs on
line-length. The inline parse (~6.5 ms) is already shed by
block-only. Output rendering (~6.8 ms on the
diagnostic-heavy run) is a formatter concern. The residual
~4.3 ms lint floor is almost entirely the block-node build
plus per-file engine overhead. This plan attacks that
floor with a flat classifier — no node tree.

`line-length` is the ideal probe. Its only AST dependency
is [`CollectCodeBlockLines`][cbl], which skips fenced and
indented code. Its per-heading limit and its table
exclusion are already byte scans. So a flat classifier
that yields the code-block line set satisfies the rule
completely.

This is the narrow, measurement-first precursor to the
full [Layer 0 plan][layer0]. It builds the classifier for
one rule and one projection. It proves the number first.
That de-risks the broader re-backing before it commits to
every Layer-0 rule.

## Tasks

1. Build a flat line classifier: one forward pass over
   `f.Lines` that records a per-line class (blank, ATX
   heading, setext underline, fence open/close, in-code,
   HTML, paragraph), the code-block line set, and
   front-matter bounds. No `ast.Node`, no heap node tree;
   allocation-lean (reuse buffers, pre-size the set).
2. Re-back [`CollectCodeBlockLines`][cbl] (and the
   heading-line / table helpers `line-length` reads) on
   the flat classifier when it is available, behind a
   default-off flag, with the AST path as fallback.
3. Add the engine parse-skip gate: when every enabled
   rule is line-capable (line-length alone, to start),
   skip [`NewFileFromSourcePooled`][newfile] and drive
   the rule from the flat classifier only.
4. Equivalence gate: diff the flat classifier's
   code-block line set against the AST-derived
   `CollectCodeBlockLines` across the neutral corpus and
   every `line-length` fixture — byte-identical, or the
   prototype is wrong.
5. Measure: hyperfine `gomarklint max-line-length` vs
   `mdsmith line-length` (flat Layer-0) on benchmark 2,
   both the pure-lint (no-diagnostic) and diagnostic-heavy
   cases, reusing the spike's single-rule configs. Record
   the numbers and an explicit go/no-go in the
   [research note][bottleneck].
6. If the pure-lint case still misses gomarklint, profile
   the flat path and attribute the remainder (classifier
   cost vs per-file engine overhead). If it clears the
   bar, note that the diagnostic-heavy case is now gated
   by output rendering (bottleneck 2) and scope a terse
   formatter as separate follow-up — out of scope here.

## Acceptance Criteria

- [x] A flat line classifier that allocates no node tree,
      with a dedicated unit test, under the per-rule alloc
      budget. (`lint.ClassifyLines` in
      `internal/lint/lineclass.go`;
      `TestClassifyLines_AllocBudget`.)
- [x] `CollectCodeBlockLines` served from the flat
      classifier is byte-identical to the AST-derived set
      across the corpus and the `line-length` fixtures
      (equivalence gate green). (Corpus gate under
      `MDSMITH_FLATL0_CORPUS`; 616-fixture CI gate
      `TestFlatClassifierEquivalence_Fixtures`; edge-case
      gate `TestFlatClassifierEquivalence_Cases`.)
- [x] With `line-length` the only enabled rule, the
      goldmark parse is skipped, proven by a test or
      profile. (`TestFlatLayer0_EquivalentDiagnostics`
      asserts `flatL0Active`; `TestNewFileFlatPooled_SkipsParse`
      pins `f.AST == nil`.)
- [x] A measured `line-length` head-to-head against
      gomarklint on benchmark 2 (pure-lint and
      diagnostic-heavy), captured in the research note,
      with an explicit go/no-go on the pure-lint case.
      (See [research note][bottleneck] "Flat Layer-0 result
      (measured)".)
- [x] All existing `line-length` fixtures pass unchanged.
- [x] All tests pass: `go test ./...` (the pre-existing
      `internal/release` PGO failures are an environment
      git-signing limitation, unrelated to this change.)
- [x] `go tool golangci-lint run` reports no issues

## Result

**Go.** A true flat Layer 0 reaches gomarklint-class speed
on pure-lint `line-length`. On the neutral corpus (4-core
box, hyperfine 25 runs):

- Pure-lint vs gomarklint's own pure-lint (both 0-diag, no
  output): **1.04x by mean, 1.07x by min** — statistical
  parity. Block-only sat at 1.79x and full parse at 2.31x
  on the same baseline.
- Against the per-rule study's gomarklint-with-output
  baseline (the ~1.26x the block-only proxy left): the flat
  path is now ~1.4x **faster**. The classifier sheds ~1.75x
  of pure-lint wall versus block-only and ~2.3x versus full
  parse — the goldmark block-node-tree build the study
  blamed for the residual.

The ~1.05x that remains is per-file engine overhead, not
the parse. That overhead is config resolution, gitignore,
the generated-section scan, front-matter parsing, and FS
setup. The diagnostic-heavy case is now gated by output
rendering (bottleneck 2), not the parse. That is a
terse-formatter follow-up, out of scope here. The broader
Layer-0 re-backing ([layer0]) is de-risked. The classifier
and its equivalence gate are the foundation it builds on.

[spike]: 2606141901_spike-block-only-parse-cost.md
[layer0]: 2606141902_lazy-parse-layer0.md
[bottleneck]: ../docs/research/benchmarks/lazy-parse-architecture.md
[cbl]: ../internal/lint/codeblocks.go
[newfile]: ../internal/lint/filepool.go
