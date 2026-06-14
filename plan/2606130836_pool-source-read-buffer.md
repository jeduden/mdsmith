---
id: 2606130836
title: Pool the per-file source-read buffer across lintFile passes
status: "✅"
model: opus
summary: >-
  bytelimit.readAllSized is the top allocator on a check run
  (~10 MB / ~10% of alloc_space on the repo corpus). Every
  workspace file's source bytes are a fresh allocation that dies
  with the File. lintFile already owns a clean release boundary
  (the arena pool's release closure), so pool the source buffer
  on that same boundary the way plan 198 pooled the parse arena.
depends-on: []
---
# Pool the per-file source-read buffer across lintFile passes

## Goal

Reuse one pooled byte buffer per worker for the file source on
`mdsmith check`. Today the engine allocates a fresh `source` slice
for every workspace file. That slice is the largest remaining
allocator the PR #600 profiling found.

## Background

An `-alloc_space` profile of `mdsmith check` over the repo
benchmark corpus ranks
[`bytelimit.readAllSized`](../internal/bytelimit/bytelimit.go) as
the top allocator. It accounts for about 10.6 MB, near 10% of all
bytes allocated on the run. The profile was taken after the PR #600
changes.

That buffer holds a file's full source bytes. The engine reads it
once per file in
[`Runner.lintFile`](../internal/engine/runner.go). The parsed
`*lint.File` then aliases it. `Source` points at it, and the
`Lines` slices come from `bytes.Split` over it.

The slice is garbage the moment the File dies. lintFile already
marks that point. The `release()` closure from
[`NewFileFromSourcePooled`](../internal/lint/filepool.go) is
deferred in lintFile. It resets and returns the parse arena.

Plan 198 set up this boundary for arena slabs. It also documented
the rule: diagnostics carry only copied strings and ints. So after
`release()` the File and its aliased memory are dead. The source
buffer has the same lifetime. It can ride the same boundary.

This is a measured allocation lever, not a CPU rewrite. The
goldmark parse and the cross-file caches dominate CPU. This plan
targets GC pressure instead. The alloc budget and the
`BenchmarkCheckCorpus*` wall-time gates both observe it.

## Design

Add a process-wide `sync.Pool` of `*[]byte` source buffers. lintFile
draws one and returns it from the same `release()` closure that
returns the arena. The read path fills a pooled buffer instead of
allocating fresh.

These constraints keep it sound:

- Only the engine's `lintFile` uses the pooled read. It is the one
  caller whose File provably dies before `release()`. Every other
  reader keeps the plain allocating read. That set is the LSP
  `ParseCache`, the `RunCache` target loads, and `mdsmith export`.
  Those callers already keep `NewFileFromSource` over the pooled
  variant for the same reason.
- The buffer returns only inside the existing `release()`. By then
  Check has returned and diagnostics are copied out. The File, its
  `Lines`, and any output aliasing `Source` are done.
- Buffers vary up to the 2 MB input cap. On `Get`, reslice to the
  needed length. Grow only when capacity is short. One large file
  must not permanently inflate every pooled buffer.
- The overflow semantics stay. The read still checks `max+1` and
  flags a too-large file. This is an allocation change only.

The cleanest seam is a new `bytelimit` entry point that fills a
caller-owned buffer (for example `ReadFileLimitedInto`). lintFile
owns the pool. The existing `ReadFileLimited` delegates to it with
a nil buffer, so its callers are unchanged.

## Tasks

1. [x] Add a failing alloc-focused benchmark or test. It pins the
   per-file source allocation count on the engine check path. The
   win is then observable and regression-gated.
   `TestReadFileLimitedInto_ReuseNoAlloc` pins the read boundary
   (`≤5` allocs steady-state); `TestRunner_PooledSourceReuseDoesNotCorrupt`
   guards the engine reslice against stale bytes.
2. [x] Add the buffer-filling read entry point in
   [`internal/bytelimit`](../internal/bytelimit/bytelimit.go). Keep
   the `max+1` overflow check. Add unit tests for the in-cap,
   at-cap, over-cap, grow-needed, and reuse cases.
3. [x] Add the `sync.Pool` of source buffers in
   [`internal/engine`](../internal/engine/runner.go). Draw in
   lintFile. Return it from the existing `release()` closure so the
   buffer and the arena share one lifetime boundary.
4. [x] Confirm no other caller is on the pooled path. Leave
   `NewFileFromSource` and plain `ReadFileLimited` callers as they
   are.
5. [x] Verify behaviour is unchanged. Run the integration fixtures,
   `go test -race ./...`, and `mdsmith check .`.
6. [x] Re-profile `-alloc_space` on both corpora. Record the
   measured drop, or a negative result, in this plan.

## Measured result

On `BenchmarkCheckCorpusLarge` (600 files, `-benchtime 20x`,
`-memprofile`):

- Baseline (allocating `ReadFileLimited`):
  `bytelimit.ReadFileLimited` accounts for **180.7 MB, 8.98%** of
  alloc_space; total run alloc_space **2012.9 MB**.
- Pooled (`ReadFileLimitedInto` + `sourceBufPool`):
  the read path falls out of the profile entirely (0 samples —
  `pprof -list readAllSized` is empty); total run alloc_space
  **1886.2 MB**, a **~126 MB / ~6.3%** drop. The residual delta
  vs the 180 MB read line is grow events folded into the warm
  pool buffer.
- `BenchmarkCheckCorpus{Small,Large}` stay within budget (Small
  p95 16 ms / 13.7 k allocs, Large p95 95 ms / 126 k allocs).

## Acceptance Criteria

- [x] The engine check path does no per-file source-buffer
      allocation in steady state. The new benchmark or test pins
      this.
- [x] Source-read bytes fall measurably on the repo-corpus
      `-alloc_space` profile. The number is recorded here.
- [x] No reader whose File outlives the call uses the pooled
      buffer. That set is the LSP ParseCache, RunCache target
      loads, and export.
- [x] Byte-limit overflow behaviour is unchanged for in-cap,
      at-cap, and over-cap files.
- [x] `BenchmarkCheckCorpus{Small,Large}` stay within budget.
- [x] `mdsmith check .` passes (generated sections in sync).
- [x] All tests pass, race-clean: `go test -race ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run` reports no
      issues.
