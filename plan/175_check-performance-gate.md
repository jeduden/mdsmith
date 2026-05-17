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
2. [x] Add `BenchmarkCheckCorpus` in
   `internal/engine/bench_test.go`: 300-file synthetic
   workspace, full production rule set, p95 vs an absolute
   6 s budget, `b.ReportMetric` for p95 and per-file cost.
3. [x] Add the `check-bench` CI job in
   [`ci.yml`](../.github/workflows/ci.yml), mirroring
   `lsp-bench`.
4. [x] Make `run.sh` the source of the cross-tool numbers;
   emit `results.fragment.md` and `headline.fragment.md`
   under `docs/research/benchmarks/`.
5. [x] Replace hand-typed tables/numbers with `<?include?>`
   of those fragments in the
   [benchmark doc](../docs/research/benchmarks/README.md),
   the [linter comparison](../docs/background/markdown-linters.md),
   and the [README](../README.md); drop the stale figure
   from the [performance feature](../docs/features/performance.md)
   and the website hero.
6. [ ] Confirm `check-bench` is green in CI and ask the
   maintainer to add it to required status checks next to
   `lsp-bench`.

## Acceptance Criteria

- [x] `BenchmarkCheckCorpus` fails (non-zero) when p95
      exceeds the budget and passes within it on a normal
      run (observable: `b.Fatalf` path exercised by a
      deliberately tiny budget locally).
- [x] No performance number in `README.md`,
      `docs/background/markdown-linters.md`, or
      `docs/research/benchmarks/README.md` is hand-typed;
      each is an `<?include?>` of a `run.sh`-generated
      fragment.
- [x] `mdsmith check .` passes (generated sections in sync).
- [ ] CI `check-bench` job passes on this branch.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
