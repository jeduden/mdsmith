---
id: 2606171400
title: "Parity parse-skip: unify the two Layer-0 gate mechanisms"
status: "🔲"
summary: >-
  Reconcile the engine's two parse-skip gates — the static
  rulelayer.IsLayer0 set (block rules) and the config-aware
  rule.LineCapable check (line rules) — into one per-file decision, so
  a config-dependent line rule like MDS001 line-length counts as
  Layer 0 under the parity config and stops forcing the parse.
model: opus
depends-on: [2606171258]
---
# Parity parse-skip: unify the two Layer-0 gate mechanisms

## Goal

Let the parse-skip gate admit rules whose Layer-0 status depends on
their configuration. Then MDS001 line-length and its peers skip the
parse under the parity config.

## Background

The engine has two parse-skip mechanisms today.

- `layer0SkipEligible` keys off the **static**
  [rulelayer](../internal/rulelayer/rule_walk_audit.json) set. It
  supports block rules through `rule.BlockChecker`, but a rule whose
  layer depends on config is marked AST-forcing for every config.
- `computeFlatLayer0Active` keys off the **config-aware**
  `rule.LineCapable` check. It handles MDS001 — line-length is
  line-capable until a per-heading limit is set — but it covers only
  line rules and builds no block spans.

A scope sweep shows MDS001 is parity-enabled yet counts as AST-forcing
to the block gate, because `rulelayer.IsLayer0("MDS001")` is false. The
parity config sets no per-heading limit, so line-length is line-capable
there. The two gates must become one decision that respects both
signals.

## Tasks

1. Resolve each enabled rule's effective config first, then ask its
   layer. A rule is skip-safe when it is `rulelayer.IsLayer0` OR its
   configured instance reports `rule.LineCapable`.
2. Replace `allEnabledRulesLayer0` with that per-file, config-aware
   check. Keep the unknown-rule-is-AST-forcing default.
3. Make the skip File serve both projections: the `ClassifyLines`
   line classes (already wired) and the block-span scan.
4. Extend `TestLayer0Gate_CorpusDiagnosticsEquivalence` to run the
   parity config (not only the Layer-0-only config), so a
   config-dependent rule's skip output is diffed against its parse
   output across the corpus.

## Acceptance Criteria

- [ ] MDS001 line-length skips the parse under the parity config and is
      byte-identical to the parse path on the corpus.
- [ ] The two gate code paths share one eligibility function.
- [ ] `go test ./...` passes.
