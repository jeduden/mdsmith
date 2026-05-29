---
id: 219
title: Multiplexed AST walk to close the parity gap to mado
status: "🔲"
model: opus
depends-on: [175, 195]
summary: >-
  Replace the per-rule goldmark AST walks with one shared
  traversal that dispatches each node to every interested
  rule, cutting the ~27% cumulative walk cost and closing
  the remaining mdsmith-parity gap to mado on long-prose
  corpora.
---
# Multiplexed AST walk to close the parity gap to mado

## Goal

Walk each file's goldmark AST once per check and dispatch
every node to the rules that care about it, instead of
having each rule call `ast.Walk` on its own. This removes
the duplicated-traversal cost the profiler named as the
last big lever.

## Background

[Plan 175](175_check-performance-gate.md) drove the
single-core engine down by roughly half, then reached a
clear verdict: no cheap win remains. Its profiler trace put
goldmark `ast.Walk` / `walkHelper` at about 27% cumulative
CPU, because every AST-walking rule re-traverses the same
tree. [Plan 195](195_per-rule-alloc-budget.md) attacked the
allocation side with a per-rule budget gate.

The [parity convention](../docs/reference/conventions.md)
now matches rumdl on both corpora and matches mado on the
repo corpus. It still trails mado by roughly 1.9x on the
longer-prose neutral corpus (see the
[benchmark](../docs/research/benchmarks/README.md)). Long
files have more AST nodes, so the per-node re-walk overhead
scales with content. The multiplexed walk is the lever the
plan-175 notes deferred as its own scoped work.

Rules reach the tree through `lint.File` and call `ast.Walk`
themselves. A shared pass walks once and calls registered
per-node handlers, so N rules cost one traversal rather than
N.

## Tasks

1. Define an opt-in visitor interface a rule may implement
   alongside `Check`: it declares the goldmark node kinds
   the rule cares about and a per-node callback that
   appends diagnostics. Rules that do not implement it keep
   their current `Check` path unchanged.
2. Build the multiplexer in the engine: one `ast.Walk` per
   file that, at each node, dispatches to every rule
   registered for that node kind. Resolve diagnostic
   ordering and dedup so output byte-matches the current
   engine.
3. Migrate the heaviest AST-walking rules first, chosen
   from a fresh profile. Each migration is behaviour-
   preserving: the rule's existing fixtures must pass
   unchanged before and after.
4. Keep line-oriented rules on `Check`; the multiplexer and
   the line pass run side by side. Document which path each
   rule uses.
5. Hold the multi-goroutine check and LSP paths race-clean
   under `-race`; the shared walk must not introduce
   cross-file or cross-goroutine state.
6. Extend the
   [check-bench gate](175_check-performance-gate.md) to
   track the win. Target: the `mdsmith-parity` neutral-
   corpus ratio to mado within about 1.2x, or a profiler
   showing no cheap win remains.
7. Refresh the benchmark prose and, if the harness is
   re-run, the committed `data/*.json` and fragments.

## Acceptance Criteria

- [ ] One AST traversal per file dispatches to all
      registered rules; no migrated rule calls `ast.Walk`
      on its own.
- [ ] Every migrated rule's fixtures and unit tests pass
      unchanged (behaviour-preserving).
- [ ] Engine diagnostic output byte-matches the pre-refactor
      output across the test corpora.
- [ ] The check-bench gate shows the neutral-corpus
      `mdsmith-parity` ratio to mado improved toward the
      target.
- [ ] `-race` is clean for the parallel check and LSP
      paths.
- [ ] `mdsmith check .` passes.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
