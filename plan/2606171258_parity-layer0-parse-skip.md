---
id: 2606171258
title: "Parity Layer-0 parse-skip: migrate the AST-forcing parity rules"
status: "🔳"
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

Measured via the engine's Layer-0 eligibility gate. Layer is the data a
rule needs, not its trigger kind:

- **Layer 0 (line / block span):** MDS002 heading-style ✅, MDS003
  heading-increment (levels), MDS004 first-line-heading (position +
  level), MDS010 / MDS011 / MDS065 / MDS066 fenced-code, MDS015
  blank-line-around-fenced-code, MDS031 unclosed-code-block, MDS069
  unique-frontmatter.
- **Layer 0 with care (list spans):** MDS014 blank-line-around-lists,
  MDS016 list-indent, MDS061 list-marker-space, MDS059
  blockquote-whitespace — list/quote spans are the divergence-prone
  case CommonMark parsing makes subtle.
- **Layer 1 (inline text):** MDS005 no-duplicate-headings, MDS017
  no-trailing-punctuation-in-heading (heading text needs inline
  flattening), MDS053 no-unused-link-definitions (reference map).

A heading rule that reads only the line is Layer 0 — its style, level,
and position are all on the line. A rule that reads the flattened
heading *text* is Layer 1. A placeholder token or a trailing emphasis
span needs the inline tree, so that rule drives the shared inline
re-parse instead.

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

## Acceptance Criteria

- [ ] Every AST-forcing parity rule resolves to Layer 0 / Layer 1.
- [ ] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with the full
      parity set enabled.
- [ ] `mdsmith check -c parity` skips the parse on benchmark 2.
- [ ] All tests pass: `go test ./...`.
