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
   so the rule flips to `A-no-skipping`. A code-content rule cannot reach
   `A-no-skipping` (the probe's code perturbation marks it
   code-sensitive); it terminates at `B-prose-only`, which `rulelayer`
   now promotes to Layer 0 when `nil_ast_safe` holds (see the closing
   section).
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

## Result

- [x] MDS002 heading-style: reads only the heading line (style + level),
      never the inline tree. `CheckBlock` serves the nil-AST path
      byte-identically, including the indented-ATX edge.
- [x] MDS013, MDS015, MDS044: confirmed byte-identical to the AST on all
      453 code-bearing corpus files (the sweep above).
- [x] Code guard removed; skip File carries `ClassifyLines`; the corpus
      equivalence gate engages on code files and is green.
- [x] All five batch plans landed
      ([2606171400](2606171400_parity-gate-unification.md),
      [2606171401](2606171401_parity-layer0-heading-rules.md),
      [2606171402](2606171402_parity-layer0-fenced-code-rules.md),
      [2606171403](2606171403_parity-layer0-list-quote-rules.md),
      [2606171404](2606171404_parity-layer1-inline-rules.md)). A scope
      sweep of the merged parity config (30 enabled rules) found **one**
      remaining gate blocker: MDS066 commands-show-output.

## Closing the all-or-nothing gate: B-prose-only is nil-AST-safe

Four migrated rules read code content: MDS041 no-inline-html, MDS050
proper-names, MDS052 no-space-in-code-spans, and MDS066
commands-show-output. Each gained a working nil-AST path. But the audit
filed them under `B-prose-only`, not `A-no-skipping`. Its probe scrambles
the very content they read, so they register as code-sensitive.

`rulelayer` mapped only `A-no-skipping` to Layer 0. Parity enables just
one of the four — MDS066; the rest are opt-in. So MDS066 alone kept
forcing the parse, and the parity set never qualified for the skip.

Yet `B-prose-only` already means the probe saw *no* nil-AST divergence on
the unperturbed fixture. (A divergent rule lands in `hybrid` instead.)
The only extra signal is code-sensitivity. The code guard that made that
a disqualifier is gone. The measurement above proved the `ClassifyLines`
and Layer-1 projections reproduce code content byte for byte.

So `rulelayer` now lifts a `B-prose-only` rule to Layer 0 when its
`nil_ast_safe` signal holds. `astProjectionConsumers` stays as the veto
for a future rule that reads an unbacked AST projection. Two guards back
the change: the corpus equivalence harness (now run over the full parity
set) and each rule's `TestCheck_NilASTMatchesAST`.

## Acceptance Criteria

- [x] Every AST-forcing parity rule resolves to Layer 0 / Layer 1
      (`TestParityRuleSetIsSkipSafe`: zero parity-enabled rules force the
      parse).
- [x] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green; added
      `TestLayer0Gate_ParityCorpusDiagnosticsEquivalence` to diff
      parse-skip vs full-parse across the corpus with the full parity set.
- [x] `mdsmith check -c parity` skips the parse on benchmark 2
      (`TestLayer0Gate_ParitySkipsParse`: the parity set lints a
      code-bearing, directive/list/quote-free file with a nil AST).
- [x] All tests pass: `go test ./...`.
