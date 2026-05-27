---
id: 175
title: CI performance gate for mdsmith check, modelled on the LSP latency gate
status: "✅"
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
10. [x] Confirm `check-bench` and `bench-fragments` are
    green in CI; ask the maintainer to add both to branch
    protection's required checks next to `lsp-bench`.
    Post-`pkg/markdown` extraction (#343) `check-bench` is
    green on this branch via the exact CI command
    (`go test -run=^$ -bench=. -benchtime=20x
    ./internal/engine/...`): Small p95 28 ms (budget 2 s),
    Large p95 200 ms (budget 12 s). `bench-fragments` is
    untouched here (no fragment/JSON/README edits). Maintainer
    ask, now three gates: add `lsp-bench`, `check-bench`, and
    the new `markdown-bench` (task 12) to branch protection's
    required checks together.
11. [x] Keep driving the `mdsmith-parity` engine gap down.
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
    Pass 2 (post-`pkg/markdown` extraction #343, this
    branch): the #343 refactor preserved the Pass-1 gains —
    single-core `BenchmarkCheckCorpusLarge` (`GOMAXPROCS=1`,
    12x) measured 962 us/file, p95 577 ms, marginally better
    than Pass 1's 1046 us/file (the memoized caches and the
    parser pool survived the move into `pkg/markdown`,
    `sync.Pool`-guarded there). One further behaviour-
    preserving win landed: `unicode.IsSpace` was ~5.5% of
    single-core CPU, called per rune of every word by
    `mdtext.CountWords` / `CountSentences` and MDS024's
    `cheapBounds`. Added `mdtext.IsSpace`, an inlinable
    ASCII fast path (`r < utf8.RuneSelf` ⇒ two integer
    compares; non-ASCII delegates to `unicode.IsSpace`),
    proven byte-for-byte equivalent to `unicode.IsSpace`
    across the entire rune domain by an exhaustive sweep
    test. Single-core Large dropped to ~910–957 us/file,
    p95 ~545–574 ms (~3–5%). Profiler verdict on remaining
    headroom: the next-largest costs are `goldmark
    ast.Walk`/`walkHelper` (~27% cum — every rule re-walks
    the AST; collapsing to one multiplexed visitor is the
    real lever but a large cross-rule refactor, not a cheap
    win), the goldmark parser internals (~18%, third-party),
    and broad GC/alloc pressure (~22%); the named
    `regexp.tryBacktrack` candidate is only ~3% and is
    spread rule-by-rule. No further *cheap* win remains;
    closing the rumdl parity gap from here needs the
    multiplexed-walk refactor tracked as its own scoped
    work, not another guard.
12. [x] Extend the gate to the public `pkg/markdown`
    library (extracted in #343). It is a cross-system
    compatibility contract with no p95 backstop of its own.
    Added tiered `BenchmarkParse{Small,Large}` in
    `pkg/markdown/bench_test.go` (≈150- / ≈3000-line
    front-mattered doc, canonical parser including the
    `<?...?>` processing-instruction block, p95 vs 10 ms /
    100 ms, `b.ReportMetric` p95 + us/KB) and the
    `markdown-bench` CI job mirroring `check-bench`. Local
    baseline Small p95 ~0.23 ms, Large p95 ~4 ms; budgets
    are ~25–40x for shared-runner flake resistance, same
    philosophy as `check-bench`. `b.Fatalf` path observed
    locally with a deliberately tiny budget.

## Acceptance Criteria

- [x] The tiered benchmarks fail (non-zero) when p95
      exceeds the budget and pass within it on a normal run
      (observable: `b.Fatalf` path exercised by a
      deliberately tiny budget locally).
- [x] The public `pkg/markdown` library has tiered parse
      p95 gates (`BenchmarkParse{Small,Large}`) wired into
      CI as `markdown-bench`; the `b.Fatalf` path is
      observable with a deliberately tiny budget locally and
      the gate passes within budget on a normal run.
- [x] No performance number in `README.md`,
      `docs/background/markdown-linters.md`, or
      `docs/research/benchmarks/README.md` is hand-typed;
      each is an `<?include?>` of a generated fragment, and
      `bench-fragments` fails on a hand-edited fragment.
- [x] `internal/profiling` writes a non-empty CPU and heap
      profile when the env vars are set and is a no-op
      otherwise; the CLI command line is unchanged.
- [x] `mdsmith check .` passes (generated sections in sync).
- [x] CI `check-bench` and `bench-fragments` pass on this
      branch.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
