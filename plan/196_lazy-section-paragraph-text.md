---
id: 196
title: Lazy SectionParagraph text — defer ExtractPlainText until a caller asks
status: "✅"
model: opus
depends-on: [195]
summary: >-
  Plan 195 cut the engine-bench allocs from 764 k to 635 k. The
  biggest remaining controllable allocator is the per-paragraph
  string materialised by astutil.buildSectionParagraphs — 1.3 M
  bytes.Buffer.String allocations across the 10-iteration
  BenchmarkCheckCorpusLarge run. paragraph-readability skips
  short paragraphs after counting their words, so the
  pre-allocated text is wasted for every paragraph under the
  minWords floor. Replace the eager Text field with a lazy
  computation keyed off a stored AST node reference, and add
  mdtext.CountWordsInNode so the gate runs without allocating.
---
# Lazy SectionParagraph text — defer ExtractPlainText until a caller asks

## Goal

Move the per-paragraph `ExtractPlainText` call out of
`astutil.buildSectionParagraphs`. The eager call runs
for every paragraph today. The lazy call runs only when
the rule actually wants text.

On the engine bench this saves ~45 k allocations per
iteration — every paragraph that paragraph-readability
skips for being under minWords. On real prose the
saving is smaller. Most prose paragraphs cross
minWords. There the change is net-zero: the same
ExtractPlainText runs, just later.

## Background

[Plan 195](195_per-rule-alloc-budget.md) tightened the
per-rule alloc budget and cut the engine bench by 17 %.
The remaining top controllable allocator is the string
copy `mdtext.ExtractPlainText` produces per paragraph.
That copy accounts for 1.3 M objects out of 8.3 M total.

The cost lives in `astutil.buildSectionParagraphs`. The
plan-195 profile traces the call site to
[`paragraphreadability.Rule.Check`][prr] via the
per-File memo.

[prr]: ../internal/rules/paragraphreadability/rule.go

`paragraphreadability.Rule.Check` reads the paragraph
text purely to compute its word count and ARI index. A
paragraph under `minWords` (default 20) is skipped
before the index runs, so its text is allocated and
discarded. The synthetic engine corpus' "This is a
synthetic sentence ..." block is 13 words long. Every
one of the 45 k paragraphs the bench parses falls
below the floor.

The other consumers of `SectionParagraph.Text` are all
opt-in:

- [paragraphstructure (MDS024)][pst] reads `p.Text`
  for sentence segmentation.
- [requiredtextpatterns (MDS057)][rtp] and
  [requiredmentions (MDS058)][rm] read `Text` through
  the [`SectionBody`][sb] helper.
- duplicated-content (MDS037) is opt-in.

[pst]: ../internal/rules/paragraphstructure/rule.go
[rtp]: ../internal/rules/requiredtextpatterns/rule.go
[rm]: ../internal/rules/requiredmentions/rule.go
[sb]: ../internal/rules/astutil/astutil.go

Eager materialisation made sense when every consumer
walked the same text. Lazy materialisation is the right
shape now that one consumer — the default-on
paragraph-readability — only needs the word count.

## Approach

Two changes in `internal/rules/astutil/`:

1. Add `Node ast.Node` to `SectionParagraph`. Stop
   computing `Text` in `buildSectionParagraphs`; the
   field stays as a documented cache (callers can
   populate it for test construction) but the engine
   sets only `Line` and `Node`.

2. Move text production to a method:

   ```go
   func (p SectionParagraph) ExtractText(source []byte) string {
       if p.Text != "" {
           return p.Text
       }
       return mdtext.ExtractPlainText(p.Node, source)
   }
   ```

   The Text shortcut keeps existing test literals
   working without forcing them to construct AST
   nodes.

One change in `internal/mdtext/`:

3. Add `CountWordsInNode(node ast.Node, source []byte) int`
   — an AST-walking word counter that produces the same
   number `CountWords(ExtractPlainText(node, source))`
   would, without materialising the string. The
   semantics across child nodes match
   `ExtractPlainText`'s concat shape: a word boundary
   is a whitespace rune or the boundary between two
   text segments whose joined run does not contain
   whitespace. Verified by an equivalence harness over
   the existing fixture corpus (every paragraph in
   `internal/rules/MDS023-paragraph-readability/`
   produces the same count both ways).

Per-rule wiring:

- **paragraph-readability** uses `CountWordsInNode`
  for the gate; calls `ExtractText(f.Source)` only for
  paragraphs that pass `minWords`.
- **paragraphstructure**, **requiredtextpatterns**,
  **requiredmentions**, **duplicatedcontent** call
  `ExtractText(f.Source)` per paragraph. Net-zero
  versus today (they always materialise).
- `SectionBody` takes `source []byte` and calls
  `p.ExtractText(source)` per paragraph.

## Tasks

1. [x] Add `mdtext.CountWordsInNode`. Cover with a
   table-driven test that pins each case the
   `extractText` switch handles (Text, String,
   CodeSpan, Image, Link, Heading, nested emphasis,
   SoftLineBreak, HardLineBreak).
2. [x] Add an equivalence harness that runs every
   paragraph in `internal/rules/MDS023-paragraph-readability/good/`
   and `bad/` through both `CountWords(ExtractPlainText(...))`
   and `CountWordsInNode(...)`; the two counts must
   agree for every paragraph.
3. [x] Add `Node ast.Node` to
   `astutil.SectionParagraph`. Update
   `buildSectionParagraphs` to set `Node` and *not*
   set `Text`. Keep the `Text` field on the struct so
   test literals continue to compile.
4. [x] Add `(SectionParagraph).ExtractText(source []byte) string`
   that returns `Text` when non-empty and falls back to
   `ExtractPlainText(Node, source)` otherwise.
5. [x] Update `SectionBody` to take `(paragraphs,
   source, start, end)` and call `ExtractText` per
   matched paragraph. Update its tests and its two
   callers.
6. [x] Update paragraph-readability to use
   `CountWordsInNode` for the `minWords` gate and
   `ExtractText` for the index calculation.
7. [x] Update paragraphstructure to call
   `p.ExtractText(f.Source)` rather than reading
   `p.Text` directly.
8. [x] Update requiredtextpatterns and requiredmentions
   for the SectionBody signature change.
9. [N/A] Update duplicatedcontent for the same change
   (it also reads paragraph text). — Verified
   inapplicable: MDS037's `extractParagraphs` walks
   the AST directly via `n.Lines()` and reads raw
   source bytes; it does not use
   `astutil.SectionParagraph` or `ExtractPlainText`.
   Nothing to change.
10. [x] Re-run BenchmarkCheckCorpusLarge and
    BenchmarkPerRuleAllocBudget. Expected on the
    synthetic corpus: allocs/op drops from ~635 k to
    ~590 k (the ~45 k paragraph-readability skips), and
    MDS023 paragraph-readability's gate number drops
    from 10 to ~7. Update the engine-bench `Allocs`
    budget and the grandfather map accordingly.
    Measured: Large dropped from ~634 k to ~553 k (a
    bigger win than projected — every per-paragraph
    string allocation is gone for the synthetic
    corpus, not just the skipped ones), MDS023 from
    10 to 8 allocs/op. Engine-bench budget tightened
    to 670 k / 70 k; no grandfather row needed (MDS023
    stays under the ≤ 10 ceiling).
11. [x] Run `go test ./...`, `go test -race ./...`,
    `go tool golangci-lint run`, `mdsmith check .`.
    Non-race suite passes in full. Under `-race`,
    `paragraphstructure.TestSentBufPool_ClearReleasesStringReferences`
    flakes — verified pre-existing by `git stash`-ing
    the plan-196 diff and re-running; the same test
    still failed on the unmodified base. Plan-196
    touched packages (mdtext, astutil,
    paragraphreadability, paragraphstructure,
    requiredtextpatterns, requiredmentions) pass
    `-race` cleanly when that one test is skipped
    via `-run` exclusion.

## Risk

The Text shortcut keeps existing tests compiling. A
rule that constructs a `SectionParagraph` literal with
only `Text` set, no `Node`, still works. But the
shortcut also hides bugs: a caller that loses the
`Node` field can silently fall back to the cached
Text string. Mitigation: this only matters for test
code. Existing test literals assert on `Text`
directly, not via `ExtractText`. The plan reads every
`SectionParagraph{...}` literal in the test corpus
before landing the rule wiring.

`CountWordsInNode` has to match the existing
`CountWords(ExtractPlainText(...))` chain byte-for-byte
on the corpus. The equivalence harness in task 2 is the
gate. Drift fails the test on the next run.

The signature change to `SectionBody` ripples through
three rule packages. Each one is small (single
function call), but a forgotten caller emits a build
error rather than a silent semantics drift.

## Acceptance Criteria

- [x] `BenchmarkCheckCorpusLarge` allocs/op drops by
      at least 30 000 (the wasted-extract bound from
      the synthetic corpus). The new lower number is
      pinned in the engine-bench `Allocs` budget.
      Measured drop ~81 000 (634 k → 553 k); budget
      moved from 760 k to 670 k.
- [x] `mdtext.CountWordsInNode` matches
      `CountWords(ExtractPlainText(...))` on every
      paragraph in the MDS023 fixture corpus.
      Pinned by the
      `TestCountWordsInNode_EquivalentToCountWordsExtractPlainText`
      harness under
      `internal/rules/paragraphreadability/`.
- [x] `BenchmarkPerRuleAllocBudget` reports MDS023
      paragraph-readability at ≤ 10 allocs/op on the
      shared fixture without a grandfather row (its
      pre-plan-196 baseline of 10 was a "just barely"
      pass). Measured 8 allocs/op.
- [x] `mdsmith check .` passes.
- [x] `go test ./...` passes in full.
- [x] `go test -race ./...` passes for every test
      this plan touched (mdtext, astutil,
      paragraphreadability, paragraphstructure,
      requiredtextpatterns, requiredmentions).
      `paragraphstructure.TestSentBufPool_ClearReleasesStringReferences`
      flakes under `-race` on main (verified
      pre-existing by an A/B with `git stash`; see
      task 11 note above); the flake is unrelated
      to this plan and is left as-is.
- [x] `go tool golangci-lint run` reports no issues.
