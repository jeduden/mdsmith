---
id: 2608020650
title: Convert MDS003/MDS005 to KindScopedChecker without a stale-state bug
status: "✅"
model: sonnet
summary: >-
  headingincrement (MDS003) and noduplicateheadings
  (MDS005) each run their own full ast.Walk instead
  of joining the engine's shared kind-scoped
  NodeChecker dispatch, because both carry
  cross-heading state (prevLevel, a seen-text map)
  that the current NodeChecker contract has no safe
  place to reset per file. This plan designs that
  reset point before converting the two rules.
---
# Convert MDS003/MDS005 to KindScopedChecker without a stale-state bug

## Goal

MDS003 (heading-increment) and MDS005
(no-duplicate-headings) are both default-enabled.
Each drives its own [`ast.Walk`][walk-hi] over every
file. That duplicates a walk the engine's shared
[`rule.KindScopedChecker`][walk-go] dispatch already
performs for dozens of sibling rules. Converting both
removes one full-tree traversal per file, for two
rules that run on every file with headings.

This plan grew out of a [High-Performance Go][hpg]
audit of origin/main. Three subagents scanned in
parallel across five categories: allocations, strings
and bytes, struct layout, skipped work, and
concurrency. This double-walk was their top hit.

## Background

Both rules need cross-node state. `headingincrement`
threads a `prevLevel` int across heading visits.
`noduplicateheadings` threads a `seen` map the same
way. [`rule.NodeChecker`][walk-go] requires the
opposite: a rule "keeps no state across nodes."

The blocker is not thread safety within one file's
walk. `runNodeCheckers` drives one file's dispatch via
a single-goroutine recursion, so that part is safe.
The real blocker is rule *lifetime*.
[`rule.CloneInstance`][clone-go] gives each worker one
long-lived rule instance. That instance processes
every file in the worker's shard, one after another.
`classifyRules` reuses a worker's existing clone
across files whenever a file needs no fresh settings.

A field on the `Rule` struct — `prevLevel`, or a
`curFile` pointer to detect a new file — would carry
state from one file into the next, unless something
resets it. `CheckNode` has no "start of file" hook.
Detecting a new file by pointer identity is not safe
either: the standalone [`rule.WalkNodes`][walk-go]
path (unit tests, the LSP) can call `Check`/`CheckNode`
more than once on the *same* `*lint.File` pointer, and
each call must independently return the correct
answer.

`blanklinearoundheadings` (MDS013) is the model for a
correct `KindScopedChecker` heading rule. Its
per-heading verdict needs no cross-heading state, so
it sidesteps this problem instead of solving it.

The same audit found four more rules shaped this way.
None are fixed here; the same blocker applies to
each. They are `blockquotewhitespace` (the MD028
half), `codeblockstyle` (MDS065), `samefileanchor`
(MDS070), and `requiredstructure` (MDS020).
`codeblockstyle` checks whole-file style consistency.

## Design options to evaluate

1. **Precompute-and-cache on `*lint.File`.** Add a
   `lint.File` memo, mirroring
   `CollectCodeBlockLines` and
   `astutil.CollectSectionHeadings`. It computes the
   full ordered diagnostic set once per file, keyed to
   the immutable `f.AST`/`f.Source` rather than to
   rule-instance lifetime. `CheckNode` then does an
   O(1) lookup per heading node. This fits the
   existing "memoize on File" convention. It costs a
   new cache field per rule — `File`'s size is pinned
   by `internal/lint/file_size_test.go` — or a small
   shared keyed-memo helper if more rules need this
   shape.
2. **A `BeginFile` (or reset) hook on `NodeChecker`.**
   Call it once per file, before the walk begins. This
   lets a rule keep sequential state safely. It is a
   broader interface change: it touches every
   `NodeChecker` call site
   (`runNodeCheckers`/`WalkNodes`), so it needs a wider
   look at whether other future stateful rules would
   want it too.
3. **Leave as-is.** If profiling after the other four
   fixes in this audit round shows the double walk is
   not hot relative to parse cost, close this plan as
   won't-fix. Record the measurement here.

## Tasks

1. Benchmark `headingincrement.Rule.Check` and
   `noduplicateheadings.Rule.Check` against
   `BenchmarkCheckCorpusLarge`, or a synthetic
   heading-heavy fixture. Confirm the second full walk
   is visible in a profile before committing to a
   design (see the [Process][hpg] section).
2. Pick one design option above. Record the choice,
   and why, here.
3. Implement the reset mechanism.
4. Convert `headingincrement` and `noduplicateheadings`
   to `KindScopedChecker`. Work red then green: write
   a test that drives one rule instance across two
   files in a row, simulating worker reuse. It must
   fail before the reset mechanism exists, and pass
   after.
5. Re-run the `internal/integration` corpus/engine
   equivalence gates. Confirm output stays
   byte-identical to the pre-change `ast.Walk` path.
6. Revisit `blockquotewhitespace`, `codeblockstyle`,
   `samefileanchor`, and `requiredstructure` as
   follow-up candidates for the same mechanism. Each
   needs its own plan — their per-rule logic is larger
   and riskier than the two rules headed here.

## Design Chosen

Option 2 — a `BeginFile` hook via the new
`rule.FileResetter` interface. Added to `internal/rule/walk.go`;
called by `rule.WalkNodes` (standalone Check path) and by
`runNodeCheckers` (engine's shared walk path) before the first
`CheckNode` call for each File.

Two implementation details required care:

1. **Nil-AST routing for MDS005.** After converting to
   `NodeChecker`, `classifySlot` dropped MDS005 on
   parse-skipped Files (not a `BlockChecker`). Fixed by
   adding `InlineCapable() bool { return true }` so
   `classifySlot` routes it to the existing `checkFromInline`
   path.
2. **Data race from shallow-copied singletons.** The alloc-budget
   test calls `r.Check` directly on singleton rule instances,
   which via `WalkNodes` → `BeginFile` sets `singleton.seen` to a
   non-nil map. `cloneRules` then shallow-copies that pointer into
   multiple worker clones; two workers calling `BeginFile`
   (`clear`) and `verdict` (write) on the shared map caused a
   concurrent-map data race. Fixed by making `BeginFile` always
   allocate a fresh map (`r.seen = make(map[string]int, 4)`)
   instead of clearing in place, so each clone gets an independent
   map immediately on its first `BeginFile` call.

## Benchmark Delta

`BenchmarkHeadingRulesTogether` (51 headings, standalone
`Check` calls, 3 × 5 s runs):

| State   | ns/op | allocs/op |
|---------|-------|-----------|
| before  |  81 k |       151 |
| after   |  92 k |       157 |

The standalone benchmark regresses ~14 % because the old
`CollectHeadingNodes` memo let two sequential `Check` calls share
heading collection; the new `WalkNodes` path does an independent
`ast.Walk` per `Check` call. In the engine's production path both
rules join the **shared** `KindScopedChecker` dispatch — one AST
walk for all NodeCheckers — which is the actual gain this plan
targets. The standalone benchmark does not capture that sharing.

## Acceptance Criteria

- [x] `headingincrement` and `noduplicateheadings`
      implement `rule.KindScopedChecker`.
- [x] A regression test proves no state leaks between
      files processed by the same rule instance.
- [x] `go test ./...` and the corpus equivalence gates
      pass unchanged.
- [x] The benchmark from Task 1 is re-run. The
      before/after delta is recorded in this plan or a
      linked PR.

[walk-hi]: ../internal/rules/headingincrement/rule.go
[walk-go]: ../internal/rule/walk.go
[clone-go]: ../internal/rule/clone.go
[hpg]: ../docs/development/high-performance-go.md
