---
summary: >-
  Measured why the Layer-0 parse-skip is a wash on benchmark 2 even after
  the list rules are made skip-capable: a quote-heavy corpus clears few
  files through the gate, and for the files that do skip the Layer-0
  projections cost about as much as the goldmark parse they replace.
  Records the dispatch bug this work fixed and the two levers the real win
  still needs.
---
# Parity parse-skip: why it is a wash on benchmark 2

This note records a head-to-head measurement of `mdsmith check -c parity`
against gomarklint 3.2.3 on the pinned 234-file neutral corpus, after a
round of work that made the parity list rules run on the parse-skipped
(nil-AST) path. The headline: the parse-skip is **neutral** on this
corpus — it does not make parity faster — and the reason is structural,
not a tuning miss.

## The numbers (4-core dev box, min of many runs)

| Run                              | wall   | vs gomarklint |
| -------------------------------- | ------ | ------------- |
| gomarklint                       | ~32 ms | 1.0x          |
| parity, parse-skip off (shipped) | ~84 ms | ~2.6x         |
| parity, parse-skip on            | ~86 ms | ~2.6x         |

The absolute numbers run higher than the published page (different,
contended hardware); the ratio is what transfers. Turning the parse-skip
on moves the wall time by less than the run-to-run noise.

## Why it is a wash, in two measured parts

**Few files clear the gate.** The parse-skip gate disqualifies any file
that may hold a block quote, which it detects as any `>` byte in the
source. The neutral corpus is Rust prose: block quotes (`> Note:`) and
`>` inside fenced code (Rust generics `Vec<T>`, `->` return arrows) are
everywhere. Only ~26 of 234 files carry no `>` at all, so only those skip
the parse. The bulk of the wall time is the large quote-bearing chapters,
and they still parse.

**For the files that do skip, the Layer-0 work costs about as much as the
parse.** A skipped file does not run goldmark, but it does run three
forward passes instead: `ClassifyLines` (the code-line projection),
`scanLayer0` (the block-span scan the heading/fence rules read), and
`listscan.Parse` (the list-structure parser the list rules read). Goldmark
is a fast single parse; three line passes plus the rules land in the same
class. The saving on the few eligible files is within noise.

This matches and sharpens the earlier
[lazy-parse spike](lazy-parse-architecture.md): a flat Layer-0 line
classifier reached gomarklint's class on the **single** `line-length`
rule, but the full parity rule set rebuilds enough projections that the
parse-free path is no longer cheaper than the parse.

## What this work did fix

The path is now **correct** for list-bearing files, which it was not
before — three real defects:

- **listscan dropped the paragraph-interruption rule at the document
  root.** An ordered marker like `2026.` in the middle of a top-level
  paragraph started a spurious list where goldmark reads lazy paragraph
  text. Fixed by tracking top-level paragraph state; corpus divergence
  fell from 8 files to 2 (both HTML-comment cases, a documented listscan
  gap).
- **The list rules' nil-AST paths were dead code in the engine.** MDS014,
  MDS016, MDS045, MDS046, and MDS061 are `NodeChecker`s with a working
  `checkLayer0` branch, but the checker dispatch routes a `NodeChecker`
  only through the AST walk; on a nil-AST File it dropped them, so their
  `Check` (and its `checkLayer0`) never ran. A forced-skip probe showed
  102 missing diagnostics from exactly these rules. Fixed with a
  `rule.LinesChecker` capability (mirroring `InlineChecker`) that routes
  such a rule to its own `Check` on a skipped File.
- **The gate excluded every list-bearing file.** With the dispatch fixed
  and listscan corpus-correct, the list guard is gone; the
  `TestLayer0Gate_CorpusDiagnosticsEquivalence` gate now engages on
  list files and stays byte-identical to the parsed path.

So the parse-skip now lints list files correctly. It is kept default-off
(an opt-in seam, `MDSMITH_LAYER0_SKIP=1`) because correct is not the same
as faster: on this corpus it is a wash.

## The two levers the real win still needs

1. **Descend into block quotes.** The block-span scanner collapses a
   quote into one span, so quote-nested headings and fences are invisible
   and any `>`-bearing file is disqualified. Until the scan emits the
   nested spans (the way it already maps nested code lines), the `>` guard
   keeps the corpus's biggest files on the parse path. This is the
   eligibility lever.
2. **Make the projections cheaper than the parse.** Three line passes per
   skipped file is the ceiling found above. The projections would need to
   fuse into one pass — or the rules read a shared classification computed
   once — for the skip to beat the parse on the full rule set.

Even with both, the [gomarklint architecture note](gomarklint-architecture.md)
bounds parity at ~1.4x gomarklint, because the rule and per-file work
alone exceed a pure line scanner. Beating gomarklint outright needs that
trim too, or a smaller parity rule set — a product decision, not an
optimization.
