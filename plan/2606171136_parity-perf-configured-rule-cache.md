---
id: 2606171136
title: "Parity perf: per-worker configured-rule cache + the Layer-0 path to gomarklint"
status: "✅"
summary: >-
  Cache the configured (cloned + settings-applied) enabled rule
  list per config signature on each engine worker, so a corpus
  that shares one config configures each Configurable rule once
  instead of once per file. Cuts standing-gate allocations ~78%
  and small-file-corpus wall time ~40%, byte-identical — but only
  ~3% on the prose-heavy benchmark-2 corpus, where parse and rules
  dominate. Records the real same-machine gomarklint ratio (~1.78x)
  and the 18-rule AST-forcing inventory the remaining gap needs.
model: opus
depends-on: [2606141904]
---
# Parity perf: per-worker configured-rule cache + the Layer-0 path to gomarklint

## Goal

Make `mdsmith check -c parity` faster on benchmark 2 — the
neutral corpus of 234 Rust Book and Reference files. This plan
trims the per-file overhead first. It then scopes the Layer-0
work the rest of the gomarklint gap needs.

## Background

The benchmark study found two halves to beating gomarklint on
parity. One: shed the goldmark parse for the parity rules
(Layer 0). Two: trim the rule and per-file pipeline below a line
scanner's. This plan delivers a piece of the second half. The
piece helps every config, not only parity.

An allocation profile of a parity check showed `classifyRules`
configuring each enabled rule **per file**. Configuring a rule
clones it and runs its `ApplySettings`. Several parity rules
compile a regexp there — for example
[MDS004](../internal/rules/MDS004-first-line-heading/README.md).
A 234-file corpus that shares one config repeated that clone and
compile 234 times. The configured rule depends only on
`(rules, effective)`, never on the file. So it is cacheable per
config signature, the run-scoped-caching pattern from
[the performance session](../docs/research/perf-parity-session.md).

## Tasks

1. Split rule configuration from per-file slot building in
   [`internal/checker`](../internal/checker/checker.go).
   `ConfigureEnabledRules` is cacheable. `CheckConfiguredRules`
   is the cheap per-file half.
2. Add a per-worker `confCache` to the engine's `runResolve`. Key
   it by the effective-config signature the effective-config
   cache already computes. Reuse the configured rules across
   every file that shares that signature.
3. Tighten the `BenchmarkCheckCorpus{Small,Large}` allocation
   budgets to lock in the win.
4. Record the AST-forcing parity inventory and the all-or-nothing
   Layer-0 gate for the follow-up.

## Result

Measured two ways, with a correction that matters. Diagnostics stay
byte-identical throughout.

**On a synthetic many-small-files corpus** (the standing engine gate
uses the same shape): parity wall **~68 ms → ~40 ms (~40%)**.
`BenchmarkCheckCorpusLarge` allocs/op fell **~553 k → ~120 k (~78%)**,
its p95 **~191 ms → ~90 ms**, Small **~57 k → ~12.7 k**.

**On the real benchmark-2 corpus** (234 Rust Book + Reference files at
the pinned commits): the wall-time change is **~3%, within noise**
(~67 ms → ~65 ms). Measured head-to-head on one machine against
gomarklint 3.2.3, parity is **~1.78×** gomarklint (64 ms vs 36 ms).
That ratio holds both before and after this change.

The gap between the two corpora is the lesson. The synthetic corpus is
many tiny files. Per-file config setup (clone, ApplySettings, regex
compile) is a large fraction there, so caching it wins big. Benchmark 2
is a few hundred large prose chapters. The parse and prose rules
dominate, and config setup is negligible, so the cache barely moves the
wall clock. The real-corpus serial split is read 3%, block parse 18%,
inline parse 22%, **rules + merge 44%**; the parse-free floor is 47%.

So this change is a **real, durable allocation win (−78%)**. It cuts GC
pressure and speeds **small-file workloads** — a docs repo of hundreds
of short files, or CI linting many files. It is **not** a material
benchmark-2 wall-time win. The real benchmark-2 levers are the parse
(40%) and the flat rule+merge cost (44%). This change touches neither.

## The remaining gap to gomarklint

The parity parse-skip gate is **all-or-nothing**. The engine
skips the goldmark parse only when *every* enabled rule resolves
to Layer 0. So no parity benchmark gain lands until the whole set
is migrated. Eighteen parity rules still force the AST, measured
via the engine's Layer-0 eligibility gate:

- Headings: MDS002, MDS003, MDS004, MDS005, MDS017
- Fenced / code: MDS010, MDS011, MDS015, MDS031, MDS065, MDS066
- Lists: MDS014, MDS016, MDS061
- Other: MDS059 blockquote, MDS069 front matter, MDS001
  line-length, MDS053 reference map

[MDS013](../internal/rules/MDS013-blank-line-around-headings/README.md)
and
[MDS044](../internal/rules/MDS044-horizontal-rule-style/README.md)
show the migration template. Each adds a `rule.BlockChecker` and
flips to `A-no-skipping` in the walk audit. Two cautions for the
follow-up. The `MDSMITH_LAYER0_SKIP` gate also stands down on any
file with a code block, because the `scanLayer0` block model does
not descend into list-item code. The code-heavy neutral corpus
therefore needs the flat `lint.ClassifyLines` path, which is
byte-identical on code-in-list. And even a free parse leaves
parity `~1.4x` gomarklint, so the rule and overhead trim this
plan starts must continue alongside the migration.

## Acceptance Criteria

- [x] Configured rules are built once per config signature per
      worker, not once per file.
- [x] Diagnostics are byte-identical to the pre-change binary, on
      both the parity and full-default configs.
- [x] The standing engine gate passes with the tightened
      allocation budgets.
- [x] All tests pass: `go test ./...`. The pre-existing
      `internal/release` PGO commit-signing failures are an
      environment limit, unrelated to this change.
- [x] `go tool golangci-lint run` reports no issues. Confirmed
      clean by CI on PR #641.
- [x] Follow-up: tracked in plan 2606171258 (parity Layer-0
      parse-skip migration).
