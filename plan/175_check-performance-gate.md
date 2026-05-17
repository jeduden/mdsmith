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
8. [x] Apply the first profiler finding: replace
   `lint.(*File).LineOfOffset`'s per-call linear rescan
   (~24% of check CPU) with a cached newline index +
   binary search. Equivalence-tested against the original
   definition; cross-tool JSON re-promoted on the
   optimized binary (repo p95 ~1.0 s → ~0.8 s, neutral
   ~1.6 s → ~0.75 s).
9. [x] Apply the second profiler finding: MDS024 called
   the Punkt sentence tokenizer per paragraph (~2 GB
   allocations over the 600-file gate corpus). Add an
   allocation-free `cheapBounds` guard that skips it when
   `terminal-punct + 1 <= MaxSentences` and total words
   `<= MaxWords` (provably no violation). Soundness pinned
   by a test; full suite + `mdsmith check .` unchanged.
   Large gate p95 ~1.7 s → ~0.8 s.
10. [ ] Confirm `check-bench` and `bench-fragments` are
    green in CI; ask the maintainer to add both to branch
    protection's required checks next to `lsp-bench`.
11. [ ] Keep driving the `mdsmith-parity` engine gap down.
    Same-rule-class mdsmith is ~1.7x slower than rumdl;
    that is engine headroom, not an accepted trade-off.
    Continue the profiler loop (next candidates: the
    goldmark walk and the regexp-backtracking hotspots)
    until parity is within ~1.2x or a profiler shows no
    cheap win remains.
    Pass 1 (trace analysis, this branch): a four-agent
    profile trace of the single-core `Run` path landed four
    behaviour-preserving wins — (a) MDS053/MDS054 no longer
    re-parse the whole file; they read link reference
    definitions from the parse `NewFile` already ran via
    `lint.File.LinkReferences`; (b) the goldmark parser is
    taken from a `sync.Pool` instead of rebuilt per file;
    (c) `CollectCodeBlockLines` / `CollectPIBlockLines` are
    memoized per `File` (was ~20 redundant AST walks per
    file); (d) per-diagnostic source-context strings are
    skipped when the caller discards them (the gate, machine
    output). Two further wins from the parallelism effort's
    single-core notes also landed: (e)
    `crossfilereferenceintegrity` memoizes its per-link
    `os.Stat` / `filepath.EvalSymlinks` in package-level
    `sync.Map`s (Syscall6 was ~5.7% flat); (f)
    `mdtext.CountWords` counts in an allocation-free rune
    scan instead of `len(strings.Fields(...))` (~0.48 GB).
    Measured single-core `BenchmarkCheckCorpusLarge`
    (`GOMAXPROCS=1`, 12x): 1677 → 1046 us/file, p95 1006 →
    627 ms (−38% cumulative). All caches `sync.Once`-,
    `sync.Pool`- or `sync.Map`-guarded so the multi-goroutine
    check / LSP path stays race-clean (verified under
    `-race`). One candidate is deliberately deferred: a
    `rule.RepoScoped`-marked `DedupeDiagnostics` skip —
    catalog/include/MDS027 also emit cross-file duplicates,
    so a safe skip needs an audited marker, not a quick
    guard.

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
