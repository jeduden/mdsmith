---
summary: >-
  Measured why the Layer-0 parse-skip is a wash on benchmark 2 even after
  the list rules are made skip-capable: a profile of the eligible-only run
  shows the skip path's cost is InlineBlocks re-parsing the content through
  goldmark per run (~51%), not the line scans (~10%), so skipping the
  block parse saves nothing. Records the dispatch bug this work fixed and
  the one lever the real win needs — a light inline scanner.
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

**For the files that do skip, the skip path re-parses the content
anyway.** This is the decisive measurement, and it isolates cost from
eligibility: restrict the corpus to the 26 files that **do** clear the
gate and benchmark only those, and parse-skip on and off are identical —
**15.5 ms either way.** A CPU profile of the skip-on run says why. The
cheap line scans the skip path was supposed to substitute for the parse
are negligible (`listscan.Parse` ~5%, `ClassifyLines` and `scanLayer0`
below the noise floor). The cost is **`lint.InlineBlocks` at ~51%** —
the shared inline projection the parity link/reference rules
(`no-bare-urls`, `link-validity`, the two reference-label rules) read on
the nil-AST path. And `InlineBlocks` is not a light scan: it splits the
body into runs and calls `parseInlineWithRefsArena` →
`markdown.ParseContextArena` on each run, i.e. a **full goldmark parse**
(block phase included — `parseBlocks` shows at ~33% of the skip-on run)
of every paragraph.

So the "parse skip" does not skip goldmark; it moves goldmark from one
whole-document parse to a per-run parse of the same content, and the line
scans ride on top. Total work is unchanged — hence the exact wash. The
block-structure scan it genuinely avoids is the small part.

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

## The one lever the real win needs

**Replace the inline projection's full goldmark parse with a light inline
scan.** The profile is unambiguous: the skip path's cost is `InlineBlocks`
re-parsing the content, not the line scans. The
[lazy-parse note](lazy-parse-architecture.md) anticipated exactly this —
Layer 1 was specified as "a targeted byte scan for links, autolinks,
images, and `[label]:` definitions/uses — **not** the full emphasis
delimiter algorithm" — but the shipped `InlineBlocks` took the easy route
and runs a real `markdown.ParseContextArena` per run instead. So the work
is not line-scan fusion (those passes are ~10% combined and already cheap);
it is building the light inline index Layer 1 was always meant to be:

- A byte scanner that extracts only what the parity inline rules read —
  links and autolinks (`no-bare-urls`, `link-validity`), images
  (`no-empty-alt-text`), and reference definitions/uses
  (`no-unused-link-definitions`, `no-undefined-reference-labels`) — plus
  the code-span ranges that suppress them. No emphasis delimiter run.
- Byte-identical to goldmark's inline result for those constructs across
  the corpus and every rule fixture, the same equivalence discipline the
  block scan already meets. Link parsing edge cases (nested brackets,
  backslash escapes, code-span suppression, reference-label folding) are
  where the risk sits.
- One holdout: `no-emphasis-as-heading` (MDS018) asks a bounded emphasis
  question (is a whole paragraph one emphasis span?) that a constrained
  detector can answer without the full delimiter algorithm; see the
  lazy-parse note's "MDS018 holdout" section.

Only when `InlineBlocks` is that light scanner does the parse-skip become
cheaper than the parse. Eligibility (descending into block quotes) is a
second, smaller step that is worthless until the inline cost is cut —
it would only widen a wash.

Even then, the [gomarklint architecture note](gomarklint-architecture.md)
bounds parity at ~1.4x gomarklint, because the rule and per-file work
alone exceed a pure line scanner. Beating gomarklint outright needs that
trim too, or a smaller parity rule set — a product decision, not an
optimization.
