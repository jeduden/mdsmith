---
id: 219
title: Multiplexed AST walk to close the parity gap to mado
status: "🔳"
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

## Status note — relationship to plans 187 and 189

The stateless half of this work already shipped. Plan 187
prototyped the multiplexed walk and added the opt-in
`rule.NodeChecker` + `rule.WalkNodes`; plan 189 finished the
sweep across every pure, stateless per-node default rule (24
rules). The engine already runs ONE shared `ast.Walk` that
feeds all those rules (see
[`internal/engine/check.go`](../internal/engine/check.go)).

What remained on `ast.Walk` were the *stateful* rules. A
stateless callback on a shared rule instance cannot express
them safely under intra-file parallelism (plan 190). They
carry a value across the walk: `heading-increment`'s
`prevLevel`, `no-duplicate-headings`' `seen` map. This plan
adds the stateful sibling interface. It then migrates those
heavy rules so the shared walk subsumes their traversals.

Design decision (the plan left the visitor shape open). The
new `rule.NodeVisitorRule` returns a *fresh per-file*
`rule.NodeVisitor`. The visitor declares the node kinds it
cares about and carries per-walk state. Fresh-per-file keeps
the state race-clean by construction. It never outlives one
walk and is never shared across goroutines. Kind declaration
lets the engine route only the relevant nodes.

## Tasks

1. [x] Define an opt-in visitor interface a rule may
   implement alongside `Check`: it declares the goldmark
   node kinds the rule cares about and a per-node callback
   that appends diagnostics. Rules that do not implement it
   keep their current `Check` path unchanged. Added
   `rule.NodeVisitor` / `rule.NodeVisitorRule` /
   `rule.WalkVisitor` in
   [`internal/rule/visitor.go`](../internal/rule/visitor.go).
2. [x] Build the multiplexer in the engine: the single
   `ast.Walk` per file dispatches to every stateful visitor
   registered for that node kind alongside the existing
   `NodeChecker` dispatch. Resolve diagnostic ordering and
   dedup so output byte-matches the current engine. The
   engine's `classifyRules` now builds a fresh per-file
   visitor for each `NodeVisitorRule` and routes its
   declared kinds through the same shared walk; per-rule
   diagnostics stay grouped in rules order, so output is
   byte-identical (pinned in
   [`multiplex_visitor_test.go`](../internal/engine/multiplex_visitor_test.go)).
3. [x] Migrate the heaviest AST-walking rules first, chosen
   from a fresh profile. Each migration is behaviour-
   preserving: the rule's existing fixtures must pass
   unchanged before and after. A fresh engine-bench profile
   named the per-rule heading walks as the cleanest remaining
   targets among default rules. Migrated the two pure
   stateful per-node default rules — MDS003 heading-increment
   (`prevLevel`) and MDS005 no-duplicate-headings (`seen`
   map). Fixtures and unit tests pass unchanged; the engine
   table-test pins each multiplexed output byte-identical to
   its sequential `Check`. Every other default rule still on
   `ast.Walk` is not a clean pure-visitor target — see "Not
   migrated" below.
4. [x] Keep line-oriented rules on `Check`; the multiplexer
   and the line pass run side by side. Document which path
   each rule uses. The path each rule takes is recorded by
   the plan-215 walk-audit manifest at
   `internal/integration/testdata/rule_walk_audit.json`:
   `is_node_checker: true` now marks both stateless
   `NodeChecker`s and stateful `NodeVisitorRule`s (the shared
   walk); line rules keep `is_node_checker: false`. The
   "Not migrated" section below records why each remaining
   `ast.Walk` rule stays on `Check`.
5. [x] Hold the multi-goroutine check and LSP paths
   race-clean under `-race`; the shared walk must not
   introduce cross-file or cross-goroutine state. The visitor
   state is fresh per file and never shared. `-race` is clean
   for `internal/engine`, `internal/rule`, `internal/lsp`,
   and the migrated rule packages.
   `TestRunner_ParallelStatefulVisitorsIsolated` pins the
   migrated visitors' per-file isolation across concurrency 1
   vs 8.
6. [x] Extend the
   [check-bench gate](175_check-performance-gate.md) to
   track the win. Target: the `mdsmith-parity` neutral-
   corpus ratio to mado within about 1.2x, or a profiler
   showing no cheap win remains. Recorded outcome:
   profiler-backed negative on wall time. A before/after A/B
   (the two heading rules' own walks vs folded into the
   shared walk) over a heading-dense 120-file corpus,
   `-count=8` single-core, measured medians of 34.6 ms
   (before) and 36.5 ms (after) — equal within run noise. The
   fresh profile shows MDS003/MDS005 no longer appear as
   separate `ast.Walk` callers; their cost moved under the
   shared-walk closure with no net change. This re-confirms
   plan 187's measured finding: the duplicated-walk *flat*
   cost (~6 %) is intrinsic per-node work, not redundant
   traversal, so folding these rules removes redundant work
   but yields no measurable wall-time win. The cross-tool
   `mdsmith-parity` harness (mado/rumdl/hyperfine) needs
   network + external installs and was not re-run here; the
   gate baselines are unchanged because no number moved.
7. [x] Refresh the benchmark prose and, if the harness is
   re-run, the committed `data/*.json` and fragments. The
   harness was not re-run (it needs network and external
   linters, and the in-process A/B shows no wall-time
   movement to promote), so the committed `data/*.json` and
   fragments are unchanged — matching plan 187 task 5's
   "nothing to promote" outcome.

## Not migrated — reason

Every default-enabled rule that still calls `ast.Walk` in its
`Check` was inspected; none is a clean pure-`NodeVisitorRule`
target:

- **MDS001 line-length**: line-level by default; its
  `collectHeadingLines` walk only runs when the non-default
  `heading-max` is set, so the default parity path never
  walks.
- **MDS051 single-h1**, **MDS065 code-block-style**,
  **MDS036 max-section-length**, **MDS037 duplicated-content**:
  collect-then-decide — a diagnostic for one node depends on
  the whole collection (the majority code-block style, all
  H1s, per-section bounds), and several also read front
  matter. MDS051/MDS036/MDS037 are opt-in, off the parity
  path.
- **MDS059 blockquote-whitespace**, **MDS062 link-validity**:
  hybrid — a line scan (MD027 marker spacing; the reversed
  `(text)[url]` regex) plus an `ast.Walk`, joined in one
  `Check` (link-validity also post-sorts). A rule is driven
  by `Check` OR by the shared walk, not both, so these cannot
  become pure visitors without splitting; their own walks are
  a fraction of a percent of CPU, so the split is not worth
  the correctness risk.
- **MDS020 required-structure**: collects headings into an
  ordered list before validating against the schema (plan 189
  already recorded this).
- **MDS050 proper-names**: uses `WalkSkipChildren` for
  correctness, then post-sorts and deduplicates (opt-in; plan
  189 recorded this).
- **MDS053 no-unused-link-definitions**, **MDS054
  no-undefined-reference-labels**: read link reference
  definitions from `lint.File.LinkReferences` (plan 175); the
  only `ast.Walk` is a small code-span-range helper, not a
  per-node pass.

## Acceptance Criteria

- [x] One AST traversal per file dispatches to all
      registered rules; no migrated rule calls `ast.Walk`
      on its own. The engine drives one shared `ast.Walk`
      that feeds every `NodeChecker` and `NodeVisitorRule`;
      the migrated rules MDS003/MDS005 delegate to
      `rule.WalkVisitor` and no longer call `ast.Walk`.
- [x] Every migrated rule's fixtures and unit tests pass
      unchanged (behaviour-preserving).
- [x] Engine diagnostic output byte-matches the pre-refactor
      output across the test corpora. Pinned per-rule by the
      `multiplex_visitor_test.go` equivalence table and
      end-to-end by the unchanged integration fixture suite.
- [x] The check-bench gate shows the neutral-corpus
      `mdsmith-parity` ratio to mado improved toward the
      target. **Resolved via the task-6 alternative — a
      profiler showing no cheap win remains.** The cross-tool
      harness needs network + external linters and could not
      be re-run here; the in-process before/after A/B over a
      heading-dense corpus shows no wall-time movement (34.6
      vs 36.5 ms median, equal within noise), re-confirming
      plan 187: the duplicated-walk flat cost is intrinsic
      per-node work, not redundant traversal. The migrations
      remove the redundant traversal and add the seam, but
      the parity ratio is unchanged by this increment. See
      task 6 for the numbers.
- [x] `-race` is clean for the parallel check and LSP
      paths.
- [x] `mdsmith check .` passes.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
