---
id: 187
title: Neutral-corpus engine lever — shared AST walk and Punkt cost
status: "🔳"
model: opus
depends-on: []
summary: >-
  Profiling the neutral Rust Book corpus shows its cost is intrinsic,
  not redundant: per-rule AST re-walks (goldmark walkHelper ~44% cum)
  and the trained Punkt sentence tokenizer behind MDS024 (~20% cum,
  the sound cheap guard already maxed). No cheap behaviour-preserving
  guard remains. Land the safe shared-paragraph-walk increment, then
  scope the two real levers: a multiplexed AST walk and a
  vetted-equivalent faster sentence segmenter.
---
# Neutral-corpus engine lever — shared AST walk and Punkt cost

## Goal

Cut `mdsmith check` wall time on prose-heavy third-party input.
The target is the 234-file neutral Rust Book and Reference
benchmark corpus. Once the catalog redundancy is removed, mdsmith
trails the per-file Rust linters there by more than on its own
directive-dense repo corpus.

## Background

A deep CPU profile of the full default rule set over the neutral
corpus ran via the env hook on this branch. It attributes the cost
to intrinsic, non-redundant work. This confirms the deferred
verdict in [plan 175](175_check-performance-gate.md) task 11, now
for the neutral corpus:

- `goldmark/ast.walkHelper` ~370 ms / ~44% cumulative: every
  default rule runs its own `ast.Walk` over the whole document.
- `paragraph-structure` (MDS024) ~210 ms / ~25% cumulative, almost
  all in `mdtext.SplitSentences` and its trained `neurosnap` Punkt
  tokenizer (~170 ms / ~20%, heavy `regexp` backtracking). The
  allocation-free `cheapBounds` guard already skips Punkt when a
  paragraph provably cannot violate a limit. Rust Book paragraphs
  exceed 40 words and run many sentences, so they cannot be skipped.
- `paragraph-readability` ~90 ms and per-file full-source regexp
  scans (`nounusedlinkdefinitions` ~70 ms) round out the profile.
  None is redundant on the check path.

The word check needs exact Punkt boundaries for its message. Punkt
only ever merges naive `[.!?]`-segments; it never splits more. So
no cheap split gives a sound upper bound on the longest sentence.
The whole-paragraph word count the guard already uses is the only
sound bound.

No cheap behaviour-preserving guard remains. The two real levers
are structural. Each is scoped work, not a one-line guard:

1. A multiplexed AST walk: one traversal that dispatches every
   rule's node visitor, instead of N full walks per file.
2. A faster sentence segmenter for MDS024, adopted only behind an
   exhaustive equivalence test against the current Punkt output so
   diagnostics (counts, previews) stay byte-for-byte identical.

## Tasks

1. [ ] Create this plan.
2. [ ] Memoize `astutil.CollectSectionParagraphs` on the per-Check
   `*lint.File` (the `File.Memo` primitive) and migrate the two hot
   default prose rules (MDS024 paragraph-structure,
   paragraph-readability) to consume it instead of each running
   their own `ast.Walk` + per-paragraph `ExtractPlainText`.
   Behaviour-preserving (integration fixtures unchanged); the
   foundation the multiplexed walk generalises. Honest measured
   note: wall time on the neutral corpus is flat here because the
   residual cost is intrinsic Punkt, not this duplicate walk — the
   value is the removed redundant CPU work and the shared seam, not
   a neutral speedup.
3. [ ] Prototype the multiplexed walk: a single `ast.Walk` driver
   that fans out to registered per-rule visitors, migrate a scoped
   batch of the default node-walking rules, and measure the
   `walkHelper` cumulative drop on both corpora. Gate behind the
   existing `BenchmarkCheckCorpus{Small,Large}` budgets.
4. [ ] Evaluate faster sentence segmentation for MDS024: build an
   exhaustive equivalence harness (Punkt vs candidate over the
   neutral corpus and the rule fixtures) and adopt a candidate only
   if diagnostics are byte-for-byte identical; otherwise record the
   negative result here and keep Punkt.
5. [ ] Re-promote cross-tool benchmark JSON on the optimized binary
   and update the gate baselines if they move.

## Acceptance Criteria

- [ ] `astutil.CollectSectionParagraphs` runs once per File;
      MDS024 and paragraph-readability share it (pinned by a
      reference-identity test); integration fixtures unchanged.
- [ ] The multiplexed walk reduces `goldmark walkHelper`
      cumulative on both corpora with no diagnostic changes across
      the full fixture suite, or is recorded here as not worth the
      cross-rule churn with the measured evidence.
- [ ] Any sentence-segmenter change is byte-for-byte
      diagnostic-equivalent to Punkt over the neutral corpus and
      the rule fixtures, or the negative result is recorded.
- [ ] `BenchmarkCheckCorpus{Small,Large}` stay within budget;
      neutral-corpus wall time improves once a structural lever
      lands.
- [ ] `mdsmith check .` passes (generated sections in sync).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
