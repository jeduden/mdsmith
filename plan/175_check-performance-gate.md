---
id: 175
title: CI performance gate for mdsmith check, modelled on the LSP latency gate
status: "🔳"
model: opus
depends-on: []
summary: >-
  Add a Go benchmark that lints a fixed synthetic workspace
  with the full rule set and fails when p95 wall time exceeds
  an absolute budget, wire it into CI as the `check-bench`
  job (mirroring `lsp-bench`), and stop hand-typing
  performance numbers in docs by feeding them from a
  benchmark-generated fragment via `<?include?>`.
---
# CI performance gate for mdsmith check, modelled on the LSP latency gate

## Goal

`internal/lsp` has a regression gate: a benchmark with a
hard p95 budget, run in CI as `lsp-bench` (plan 121).
`mdsmith check` had none. Performance claims were hand-typed
prose and drifted — a "<300 ms full check" line was off by
~5x against the real ~1.4 s full-repo time.

This plan gives `check` the same kind of gate, and makes the
documented numbers come from a real run instead of memory.

## Background

The LSP gate lives in `internal/lsp/bench_test.go`: it
builds a synthetic document, drives the real path, computes
p95, and calls `b.Fatalf` when the budget is missed. CI runs
`go test -run=^$ -bench=. -benchtime=20x ./internal/lsp/...`.

`engine.Runner.Run` is the function `mdsmith check` drives.
A benchmark over a fixed synthetic corpus is deterministic
and needs no network, so it fits CI. The cross-tool
comparison (mado, rumdl, panache, markdownlint-cli2) needs
network and external installs, so it stays a hand-refreshed
research artifact, not a CI gate.

## Tasks

1. [x] Create this plan.
2. [x] Add tiered `BenchmarkCheckCorpus{Small,Large}` in
   `internal/engine/bench_test.go` (60 / 600 files, full
   rule set, p95 vs 2 s / 12 s, `b.ReportMetric`).
3. [x] Add the `check-bench` CI job in
   [`ci.yml`](../.github/workflows/ci.yml), mirroring
   `lsp-bench`.
4. [x] Make `run.sh` promote fresh hyperfine JSON into
   `docs/research/benchmarks/data/` and call the shared
   `gen_fragments.py`; commit the JSON as the source of
   truth.
5. [x] Replace hand-typed tables/numbers with `<?include?>`
   of those fragments in the
   [benchmark doc](../docs/research/benchmarks/README.md),
   the [linter comparison](../docs/background/markdown-linters.md),
   and the [README](../README.md); drop the stale figure
   from the [performance feature](../docs/features/performance.md)
   and the website hero.
6. [x] Add the `bench-fragments` drift gate: regenerate
   from committed JSON, `mdsmith fix`, `git diff
   --exit-code`.
7. [x] Add the env-gated profiler
   (`internal/profiling`, called from `cmd/mdsmith`) and
   `profile.sh` so a tripped gate can be traced to a
   function, not just detected.
8. [ ] Confirm `check-bench` and `bench-fragments` are
   green in CI; ask the maintainer to add both to branch
   protection's required checks next to `lsp-bench`.

## Acceptance Criteria

- [x] The tiered benchmarks fail (non-zero) when p95
      exceeds the budget and pass within it on a normal run
      (observable: `b.Fatalf` path exercised by a
      deliberately tiny budget locally).
- [x] No performance number in `README.md`,
      `docs/background/markdown-linters.md`, or
      `docs/research/benchmarks/README.md` is hand-typed;
      each is an `<?include?>` of a generated fragment, and
      `bench-fragments` fails on a hand-edited fragment.
- [x] `internal/profiling` writes a non-empty CPU and heap
      profile when the env vars are set and is a no-op
      otherwise; the CLI command line is unchanged.
- [x] `mdsmith check .` passes (generated sections in sync).
- [ ] CI `check-bench` and `bench-fragments` pass on this
      branch.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
