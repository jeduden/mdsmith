---
id: 2606171258
title: "Parity Layer-0 parse-skip: migrate the AST-forcing parity rules"
status: "✅"
summary: >-
  Move every parity-active rule that still forces the goldmark parse onto
  the Layer-0 / Layer-1 projections so `mdsmith check -c parity` can skip
  the parse on benchmark 2. The parse-skip gate is all-or-nothing, so no
  benchmark gain lands until the whole set migrates; this plan tracks the
  rule-by-rule effort, starting with heading-style (MDS002).
model: opus
depends-on: [2606141904]
---
# Parity Layer-0 parse-skip: migrate the AST-forcing parity rules

## Goal

Let `mdsmith check -c parity` shed the goldmark parse on benchmark 2 (the
234-file neutral corpus), where the parse is ~40% of the check. The
engine already skips the parse when every enabled rule resolves to
Layer 0; this plan migrates the parity rules that still force it.

## Background

Measured head-to-head against gomarklint 3.2.3 on the real corpus,
parity runs at ~1.8x gomarklint. The parse is the largest single
bucket. The lazy-parse infrastructure is already in place. It has the
Layer 0 block scan ([layer0.go](../internal/lint/layer0.go)), the
`rule.BlockChecker` seam, the Layer 1 inline index, and the `rulelayer`
audit gate. MDS013 (blank-line-around-headings) and MDS044
(horizontal-rule-style) are the proven template.

The gate is **all-or-nothing**: the parse is skipped only when *every*
enabled rule is Layer 0, so the benchmark does not move until the whole
parity set migrates. Each rule is still a correct, independently-gated
increment.

## The AST-forcing parity rules

A scope sweep found 28 parity-enabled rules still forcing the parse.
That is beyond MDS002 ✅, MDS013 ✅, MDS015 ✅, and MDS044 ✅, which are
already migrated. The 28 are decomposed into five batch plans, grouped
by the data each rule reads. Layer is the data a rule needs, not its
trigger kind:

- Gate unification ([2606171400](2606171400_parity-gate-unification.md)):
  MDS001 line-length, plus the mechanism that lets a config-dependent
  line rule count as Layer 0.
- Layer-0 heading + front matter
  ([2606171401](2606171401_parity-layer0-heading-rules.md)): MDS003,
  MDS004, MDS051, MDS069.
- Layer-0 fenced code
  ([2606171402](2606171402_parity-layer0-fenced-code-rules.md)): MDS010,
  MDS011, MDS031, MDS065, MDS066.
- Layer-0 list + blockquote
  ([2606171403](2606171403_parity-layer0-list-quote-rules.md)): MDS014,
  MDS016, MDS045, MDS046, MDS061, MDS059, MDS067.
- Layer-1 inline
  ([2606171404](2606171404_parity-layer1-inline-rules.md)): MDS005,
  MDS017, MDS041, MDS042, MDS049, MDS050, MDS052, MDS053, MDS063,
  MDS068, MDS034.

A heading or fence rule that reads only its own line is Layer 0. A rule
that reads flattened inline *text* — heading text, link text, an
emphasis run — is Layer 1 and drives the shared inline re-parse.

**Known blocker (found while scoping the heading batch).** `scanLayer0`
emits heading spans only for top-level headings; it does not descend into
list-item or blockquote bodies. So any nil-AST path that walks heading
spans diverges from the AST for a heading nested in a container. The
heading and single-h1 rules cannot migrate until the scanner emits nested
heading spans (or the gate excludes container-nested headings). See
[2606171401](2606171401_parity-layer0-heading-rules.md).

## Tasks

1. Per rule: add the nil-AST path (a `rule.BlockChecker` `CheckBlock` for
   per-block rules, or a span/line walk for standalone `Check` rules),
   reusing the existing extraction where possible.
2. Regenerate the walk audit (`MDSMITH_REGEN_WALK_AUDIT=1`) and sync the
   embedded [rulelayer copy](../internal/rulelayer/rule_walk_audit.json)
   so the rule flips to `A-no-skipping`.
3. Confirm `TestLayer0Gate_CorpusDiagnosticsEquivalence` stays green — it
   enables every Layer-0 rule and diffs parse-skip vs full-parse across
   the corpus, so a divergent rule fails it.
4. ~~Once the list/quote rules land, drop the gate's code-block guard~~
   Done early — see the measurement below.

## Measurement: the code guard was the wrong fear

A full corpus sweep (1046 files, 453 code-bearing) compared every
Layer-0 rule's AST output against its nil-AST output, with front matter
stripped on both sides as the engine does. Result: **one** divergence,
not the feared code-in-list class. `scanLayer0`'s block spans and the
flat `ClassifyLines` code-line projection both already match goldmark on
code, including fences and indents inside list items. The lone
divergence was MDS002 on a ≤3-space-indented ATX heading: the AST path's
`lineStartsWithHash` reads column 1 without skipping indent, so it
flagged a heading scanLayer0 (correctly) treats as ATX. Fixed by keying
MDS002's `CheckBlock` `isATX` off the raw first byte too.

So the coarse `SourceMayHaveCodeBlock` guard bailed on any fence, tab, or
four-space run. That was far more conservative than the scanner needed.
The guard is now gone. The skip File carries the `ClassifyLines`
projection (via `NewFileFlatPooled`), so code-line rules serve the
validated classifier. The corpus equivalence gate now engages on
code-bearing files and stays green.

This does not by itself speed up benchmark 2: `MDSMITH_LAYER0_SKIP` is
still default-off, and the gate still requires *every* enabled rule to
be Layer 0 (the remaining migration). But it removes the blocker that
framed parse-skip on code as a scanner rewrite — it was a one-line rule
fix plus a guard removal.

## Result so far

- [x] MDS002 heading-style: reads only the heading line (style + level),
      never the inline tree. `CheckBlock` serves the nil-AST path
      byte-identically, including the indented-ATX edge.
- [x] MDS013, MDS015, MDS044: confirmed byte-identical to the AST on all
      453 code-bearing corpus files (the sweep above).
- [x] Code guard removed; skip File carries `ClassifyLines`; the corpus
      equivalence gate engages on code files and is green.

## Closing the set: MDS066

The five batch plans landed every rule's nil-AST path. But one rule still
forced the parse under parity: MDS066 commands-show-output. The walk audit
marks it `B-prose-only`, because it lints the fenced-code *body* (a block of
`$ ` prompts) rather than ignoring it. The static category withholds Layer 0
from any code-content-sensitive rule. So `IsLayer0(MDS066)` stayed false and
the all-or-nothing gate kept parsing.

MDS066 is nonetheless parse-skip-safe: its `CheckBlock` reads the
`BlockFencedCode` span's body straight from `f.Lines`, which the Layer 0
scan delimits exactly as goldmark does. A corpus sweep confirmed its
nil-AST output is byte-identical to the AST output on all 1054 files (it
fires on one). MDS066 is the only `B-prose-only` rule parity enables —
MDS041, MDS050, MDS052 are all disabled by the convention — so it was the
lone blocker.

The fix generalizes the layer mapping rather than special-casing MDS066.
`rulelayer.nilASTBackable` promotes the whole nil-AST-safe `B-prose-only`
category to Layer 0, gated on the manifest's `nil_ast_safe` signal. A
`B-prose-only` rule already matched the AST run on its fixture; a
divergent rule lands in `hybrid`. The only withheld signal was
code-sensitivity. The validated `ClassifyLines` and Layer-1 projections
now reproduce that code content. This admits MDS066, parity's lone
blocker, plus the opt-in MDS041, MDS050, and MDS052.

`astProjectionConsumers` stays as the veto for a future rule that reads an
unbacked AST projection. Three guards back the promotion.
`TestBProseOnlyRulesAreNilASTSafe` pins that every `B-prose-only` entry is
`nil_ast_safe`. Each rule's `TestCheck_NilASTMatchesAST` checks its own
nil-AST path against the AST path. For MDS066 — the only one parity
enables — `TestParityConvention_SkipsParse` drives the nil-AST path on a
skip-eligible file and asserts it fires.

## Acceptance Criteria

- [x] Every AST-forcing parity rule resolves to Layer 0 / Layer 1.
- [x] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with the full
      parity set enabled.
- [x] `mdsmith check -c parity` skips the parse on benchmark 2
      (`TestParityConvention_SkipsParse` and
      `TestParityConvention_AllEnabledRulesSkipSafe`).
- [x] All tests pass: `go test ./...`.
