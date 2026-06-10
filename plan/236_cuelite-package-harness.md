---
id: 236
title: "cuelite phase 0 — package, façade, and differential harness"
status: "✅"
model: opus
summary: >-
  Create the public cue/cuelite package — the Value type, the
  CUE delegation pattern, the differential harness (in-house
  path versus the CUE-backed path as oracle), and the benchmark.
  Surface façade methods and call-site migration come in the
  per-surface phases that follow.
depends-on: [215]
---
# cuelite phase 0 — package, façade, and differential harness

## Goal

Stand up the public `cue/cuelite` package as a CUE-backed
scaffold. Add the differential harness and benchmark the later
phases rely on.

## Context

Phase 0 of [plan 218](218_wasm-size-reduction.md); see it for
the full design and strategy. The façade will mirror the CUE
calls mdsmith makes, each delegating to `cuelang.org/go`. Its
methods are added in the per-surface phases. Behaviour matches
CUE, so adopting it later stays green.

## Tasks

1. Create `cue/cuelite` with its `Value` type (wrapping a
   `cue.Value`), the CUE delegation pattern, and path-tagged
   errors. Phase 0 ships the minimal surface the harness and
   benchmark need — `Compile`, `CompileJSON`, `Value.Unify`,
   `Value.Validate`, and the package-level `Errors` accessor.
   The per-surface façade methods (`ParsePath`, `LookupPath`,
   `Decode`, …) are added in their own phases. One unit test per
   function.
2. Build the differential harness: run a value or expression
   through the in-house path and the CUE-backed path, and
   assert identical accept/reject and error field-paths. There
   is no in-house path yet, so it starts as a scaffold.
3. Add the `cue/cuelite`-versus-CUE benchmark.
4. Register `cue/cuelite` in the
   [layering map](../docs/development/architecture/index.md).

## Acceptance Criteria

- [x] `cue/cuelite` is a public, exported, documented package
      with its `Value` type and delegation scaffold.
- [x] Each function ships with a dedicated unit test.
- [x] The differential harness and the benchmark run in CI.
- [x] No mdsmith call site imports `cue/cuelite` yet.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.

## Implementation notes

Three choices differ from the sketch in plan 218:

- **Per-`Value` context, not a shared package context.** An
  earlier draft compiled every `Value` against one package-wide
  `*cue.Context`. Code review rejected it: CUE v0.16.1 documents
  that values from one `Context` are not safe for concurrent use
  and that long-lived contexts grow unbounded, so a single
  process-wide context is both a data race and a memory leak.
  Instead each `Compile`/`CompileJSON` owns a fresh `*cue.Context`
  and keeps its source bytes; `Unify` rebuilds whichever side
  retains source into the OTHER side's context, so unification
  stays single-context and the result lives in the context of the
  side that was not rebuilt. That side's context is therefore
  MUTATED by the cross-context `Unify` (the rebuilt operand is
  compiled into it), so two `Value`s sharing a context are not
  concurrency-safe and a long-lived `Value` accumulates one
  compiled document per cross-context `Unify`. This is the honest
  interim cost — one context per compiled `Value`, one rebuild of
  a cross-context operand per `Unify`, and the caller's duty to
  synchronize or per-goroutine-compile a shared schema. The flip
  to the in-house engine drops contexts entirely, with no API
  change: `Value` is a value type whose `Unify` takes and returns
  a `Value`, and a bottom (⊥) absorbs in either implementation.
- **Harness lives in `internal/cuelitetest`, not under
  `cue/cuelite/`.** An earlier draft put it in a public
  `cue/cuelite/difftest` sub-package. Code review rejected that
  too: the harness imports `cuelang.org/go` from non-test files,
  so as a public package it would let external users depend on a
  package plan 218 phase 4 deletes, and it would pin `cuelang.org`
  into `go.mod` even after the flip. Moving it under `internal/`
  keeps it importable by every module test, invisible outside the
  module, and freely deletable. It exposes `CueLitePath` (the
  in-house path), `OraclePath` (the CUE oracle), `Run` over a
  `Case` corpus, and a CI-visible `TestRun_corpus`. Each `Outcome`
  carries a `Stage` discriminator (compile-schema / compile-data /
  validate / accepted / error) so a schema the in-house engine
  cannot parse can never look like agreement with an oracle that
  merely rejected the data.
- **`CompileJSON` enforces a strict-JSON, no-duplicate-keys
  contract**, stricter than the plan-218 sketch's `// CompileBytes`
  annotation implied. CUE's JSON lift unifies same-named object
  keys into a phantom merged object that no last-wins JSON reader
  would build, so both arms reject any duplicate key before the
  lift — `cuelite` with a parity-stack scanner, the oracle with an
  independent recursive walk. Both also defer lossy-decode keys
  (invalid UTF-8, lone-surrogate escapes) to the CUE lift rather
  than fabricating a duplicate, and surface a post-build bottom (a
  lone-surrogate value) as a data-stage compile error.

The phase-0 surface is small. It has `Compile`, `CompileJSON`,
`Value.Unify`, and `Value.Validate`, all on a value-type `Value`
that carries a bottom (⊥) for compile failures so a nil receiver
never panics. `Validate` returns one `*PathError` per failing leaf
(joined with `errors.Join` when several fail), each tagged with
its field path printed exactly once. The `PathError` type exports
`Path` and `Error`; its constructor is unexported, since no caller
outside the package builds one. The rest of the façade arrives in
the per-surface phases.

- **The benchmark is gated, not just recorded.** `TestFactorGate`
  in `internal/cuelitetest` computes the cuelite/cue ns-per-op
  RATIO for the hot (`BenchmarkValidate`) and cold
  (`BenchmarkCompileValidate`) paths — minimum of five fixed-loop
  runs after a discarded warmup, so the ratio cancels runner noise
  the way it cancels runner speed (the
  [benchcheck](../internal/release/benchcheck.go) philosophy) — and
  FAILS when either exceeds its interim budget: `HotFactorBudget`
  2.5x, `ColdFactorBudget` 2.0x. The hot budget is looser because
  the CUE-backed arm's cost is N-dependent (one compiled document
  accumulates in the long-lived schema context per iteration), so
  it measures ~1.9x against the cold path's stable ~1.4x. The
  ratio is only meaningful on a quiet runner — under the parallel
  `test` job's CPU contention the cuelite arm degrades more than
  the oracle arm and the factor inflates (observed 3.46x) — so the
  gate arms only when `CUELITE_FACTOR_GATE=1`, set by the dedicated
  `cuelite-bench` CI job, and skips everywhere else. The gate
  appends a factor table to `GITHUB_STEP_SUMMARY`. This
  makes plan 218's "the schema validate path does not regress"
  acceptance criterion enforceable today; plan 240's flip is
  expected to tighten both budgets to <= 1.0x (in-house must not be
  slower than the CUE oracle it replaces).

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
- [Plan 215 — engine API and WASM bindings](215_engine-api-wasm.md)
