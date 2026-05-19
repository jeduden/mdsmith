---
id: 190
title: Intra-file rule parallelism for non-NodeChecker rules
status: "🔳"
model: opus
depends-on: [187, 189]
summary: >-
  Run the non-NodeChecker subset of a file's rules concurrently within
  one file when the file-level worker pool has not saturated the
  available cores. Reverses plan 187's task 5 deferral — small-file
  LSP / PR runs leave most cores idle, so this is a free win on the
  exact paths users feel most.
---
# Intra-file rule parallelism for non-NodeChecker rules

## Goal

Parallelize the non-NodeChecker subset of a single file's rules when
the file-level worker pool leaves cores idle. Examples include
`mdsmith lsp` linting one file, or a 5-file PR check on a 16-core
host. Each rule's Check is independent: rule clones are per-call, the
File is read-only, and `File.Memo` is `sync.Map`-safe. Diagnostics
remain byte-identical to the sequential path because slots are
concatenated in rules order after the workers join.

## Background

[Plan 187](187_neutral-corpus-engine-lever.md) task 5 deferred this
work because the multiplex experiment was flat. A follow-up review
judged the compound impact of small wins is the way to close the
gap to faster Rust linters. An intra-file boost on the small-file
LSP path lands directly on user-visible latency, not on batch wall
time alone.

Concurrency model:

- The file-level pool (`Runner.runFiles`) decides its worker count
  in `ResolveWorkers`. Pass that count into `checkRules` via a new
  parameter so the inner pool can decide how many cores remain.
- Each non-NodeChecker rule's `Check(f)` writes to its own slot only;
  results merge in rules order after the workers join, so output is
  byte-identical to serial.
- `Runner.IntraFileConcurrency` gates the behaviour: 0 = auto, 1 =
  disabled, n>1 = explicit cap. Auto picks
  `max(1, GOMAXPROCS / max(1, fileWorkers))`.
- `RunSource` (LSP single-file path) uses `GOMAXPROCS` directly —
  there is no file-level pool to compete with.

## Tasks

1. [x] Write this plan.
2. [x] Add `Runner.IntraFileConcurrency` (int, default 0 = auto). Thread
   the file-worker count from `runFiles` into `lintFile` and
   `checkRules`. Compute the auto cap once per Run.
3. [x] Refactor `checkRules` to run non-NodeChecker slots concurrently
   when the cap > 1, preserving slot order at the end. Keep the
   NodeChecker multiplex unchanged (a single shared walk is already
   internally serial).
4. [x] Add tests in `internal/engine/intrafile_test.go`:
       `TestCheckRules_ParallelEqualsSequential` mirroring the
       multiplex byte-identity test; `TestCheckRules_ParallelRespectsRulesOrder`
       to pin slot order; `TestCheckRules_ParallelHonorsCap` using an
       atomic concurrency counter inside a stub rule.
5. [x] Run `go test -race ./internal/engine/...` to confirm
       race-cleanness.
6. [x] Extend `BenchmarkCheckCorpusSmall`/`Large` numbers or add a
       small-file-count variant that exercises the intra-file path
       (few files, many cores). Record before/after in the plan.

## Acceptance Criteria

- [x] `Runner.IntraFileConcurrency` field exists with the documented
      semantics (0=auto, 1=off, n>1 explicit cap).
- [x] Auto cap formula in code:
      `max(1, GOMAXPROCS / max(1, fileWorkers))` for `Run`, and
      `GOMAXPROCS` for `RunSource`.
- [x] Diagnostic output remains byte-identical to the sequential
      path on every existing fixture.
- [x] New tests pass under both `go test ./internal/engine/...` and
      `go test -race ./internal/engine/...`.
- [x] `mdsmith check .` passes and `go tool golangci-lint run` reports
      no issues.
- [x] `BenchmarkCheckCorpus{Small,Large}` stays within budget.

## ...

<?allow-empty-section?>
