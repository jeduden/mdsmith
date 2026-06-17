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
4. Once the list/quote rules land, drop the gate's code-block guard (the
   scanner must match the AST on code-in-list first) and re-measure
   parity vs gomarklint.

## Result so far

- [x] MDS002 heading-style: reads only `span.Kind` (ATX vs setext) and
      the leading-`#` level, never the inline tree. `CheckBlock` serves
      the nil-AST path byte-identically; the corpus gate is green with
      MDS002 enabled.

## Acceptance Criteria

- [ ] Every AST-forcing parity rule resolves to Layer 0 / Layer 1.
- [ ] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with the full
      parity set enabled.
- [ ] `mdsmith check -c parity` skips the parse on benchmark 2.
- [ ] All tests pass: `go test ./...`.
