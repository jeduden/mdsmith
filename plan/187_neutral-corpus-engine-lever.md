---
id: 187
title: Neutral-corpus engine lever — shared AST walk and Punkt cost
status: "✅"
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

1. [x] Create this plan.
2. [x] Memoize `astutil.CollectSectionParagraphs` on the per-Check
   `*lint.File` (the `File.Memo` primitive) and migrate the two hot
   default prose rules (MDS024 paragraph-structure,
   paragraph-readability) to consume it instead of each running
   their own `ast.Walk` + per-paragraph `ExtractPlainText`.
   Behaviour-preserving: the full integration fixture suite,
   `go test ./...`, and `mdsmith check .` are unchanged. This is
   the foundation the multiplexed walk generalises. Honest
   measured note: neutral-corpus wall time is flat here because the
   residual cost is intrinsic Punkt, not this duplicate walk — the
   value is the removed redundant CPU work and the shared seam, not
   a neutral speedup. The next two tasks hold the real levers.
3. [x] Prototyped the multiplexed walk.

   Added the opt-in `rule.NodeChecker` and `rule.WalkNodes`. The
   engine runs one shared walk for all of them. Each rule's
   diagnostics stay grouped in rules order. A test proves the
   output is byte-identical to sequential `Check`.

   Migrated a verified batch of three pure per-node default rules
   (MDS002, MDS010, MDS009). Fixtures, the suite, and the gate are
   unchanged.

   The measured result is no gain. Neutral held at 280 ms. Repo
   held at 410 ms. Both deltas are noise.

   The profile explains why. The shared-walk cumulative cost fell a
   lot. Its flat cost did not move (~6%). So that 44% was per-node
   rule work counted under the walk. It was never redundant
   traversal.

   Only the small flat cost is removable. Removing it would touch
   nearly every walking rule. That is a big churn for a tiny gain.

   So keep the primitive and the batch. Skip the full migration.
   This confirms the plan 175 task 11 deferral with numbers.
4. [x] Evaluated faster sentence segmentation for MDS024.

   Built a reusable equivalence harness and a cost benchmark in
   [the mdtext segmenter test](../internal/mdtext/sentence_equivalence_test.go).

   Recorded an evidence-backed negative. The guessed bottleneck was
   a per-call regexp recompile in the tokenizer. It does not exist.
   That function has no caller, and no compile frame shows in the
   profile.

   The real cost is the trained-model regexp run over tokens. That
   is its algorithm, not a fixable recompile. A naive splitter is
   not equivalent: it breaks abbreviations and decimals. No pure-Go
   Punkt-compatible faster segmenter exists.

   Keep Punkt. The harness stays as the cheap gate for any future
   candidate.
5. [x] Re-promote cross-tool benchmark JSON only if numbers move.
   They did not: the multiplex is flat and the segmenter is
   unchanged. Nothing to promote; gate baselines unchanged.

## Acceptance Criteria

- [x] `astutil.CollectSectionParagraphs` runs once per File;
      MDS024 and paragraph-readability share it (pinned by a
      reference-identity test); integration fixtures unchanged.
- [x] The multiplexed walk reduces `goldmark walkHelper`
      cumulative on both corpora with no diagnostic changes across
      the full fixture suite, or is recorded here as not worth the
      cross-rule churn with the measured evidence. Recorded: cum
      fell but flat (~6%) did not; full migration not worth it.
- [x] Any sentence-segmenter change is byte-for-byte
      diagnostic-equivalent to Punkt over the neutral corpus and
      the rule fixtures, or the negative result is recorded. Negative
      recorded with a reusable equivalence harness; Punkt kept.
- [x] `BenchmarkCheckCorpus{Small,Large}` stay within budget
      (Small p95 27 ms / 2 s, Large p95 189 ms / 12 s). No
      structural lever paid off, so neutral wall time is unchanged;
      that negative is the recorded outcome of tasks 3 and 4.
- [x] `mdsmith check .` passes (generated sections in sync).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
