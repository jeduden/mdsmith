---
id: 192
title: Run-scoped read cache for catalog cross-host redundancy
status: "✅"
model: opus
depends-on: []
summary: >-
  The per-Check catalog memo collapsed the three-times-per-directive
  rebuild, but different host files whose catalogs glob the same docs
  tree still re-read and re-parse those targets once per host-file
  Check. Add a run-scoped read/parse cache (front matter + include
  adjacency) keyed by path, invalidated per LSP document change, so
  the repo corpus stops paying O(host-files x shared-targets).
---
# Run-scoped read cache for catalog cross-host redundancy

## Goal

The catalog rule re-reads and re-parses each matched target once
per host file whose catalog globs it. Cut that to once per run. On
the repo corpus, `CLAUDE.md`, `PLAN.md`, and ~30 `docs/**/index.md`
files carry catalogs over the same `docs/**` tree. Each target's
front matter and include list is recomputed once per host-file
Check.

## Background

A per-Check memo already shipped on this branch. It added
`File.Memo`, `cachedCatalogEntries`, `includeTargetsOf`, and
`cachedFrontMatter`. It removed the three-builds-per-directive
redundancy inside one Check. The catalog rule fell from ~280 ms to
~140 ms cumulative on the 569-file repo corpus.

That memo lives on the per-Check `*lint.File`. It is discarded with
the file. So it does not dedupe across host files. File A's catalog
over `docs/**` and file B's catalog over `docs/**` still each read
`docs/X.md`.

Closing that needs a cache that lives for the whole `mdsmith check`
run. The hazard is the LSP. A long-lived server must not serve a
stale target after the user edits it.

The cross-file-reference rule hits the same tension. It uses a
per-Check local cache. A run cache instead needs an explicit
invalidation seam. The LSP calls that seam on `didChange` or
`didSave`.

A one-shot `mdsmith check` sees an immutable corpus, so the cache
is trivially safe there. Only the LSP path needs the hook.

## Tasks

1. [x] Add a run-scoped cache type (path -> parsed front matter,
   path -> resolved include adjacency) owned by the engine
   `Runner` and threaded to rules via a context value or a field
   on `*lint.File` that points at the shared cache (not a package
   global — testability and LSP isolation).
   `lint.RunCache` in `internal/lint/runcache.go`; the engine
   `Runner.RunCache` field threads it to each `*lint.File`.
2. [x] Route `cachedFrontMatter` and `includeTargetsOf` through
   the run cache when present, falling back to the per-Check memo
   when absent (struct-literal `*lint.File` in unit tests).
   The catalog rule now keys by absolute path; the include-cycle
   scan has an absolute-path variant (`scanIncludesForTargetAbs`)
   that uses the run cache; struct-literal Files keep the legacy
   per-Check path.
3. [x] Add an `Invalidate(path)` seam; call it from the LSP
   document-sync path so an edited file's cached read is dropped.
   Unit-test that a second Check after Invalidate re-reads.
   `Server.invalidateCachedRead` fires on `didChange`, `didSave`,
   and `didChangeWatchedFiles`;
   `TestRunCache_InvalidateForcesReread` and
   `TestRunCache_InvalidateForcesRebuild` pin the seam.
4. [x] Benchmark the repo corpus before/after with the existing
   interleaved-median harness; record the delta here. Confirm the
   neutral corpus (no directives) is unchanged and
   `BenchmarkCheckCorpus{Small,Large}` stays within budget.
   Interleaved median of 7 `mdsmith check .` runs on the actual
   repo (4-core dev box, 331 files): baseline **462 ms** ->
   with run cache **440 ms** (~5% faster, ~22 ms saved on top of
   the prior per-Check memo's 140 ms catalog cost). Neutral
   corpus is unchanged (no directives -> cache is dormant);
   `BenchmarkCheckCorpus{Small,Large}` p95 stays well within
   budget (Small ~28 ms / 2 s budget; Large ~197 ms / 12 s).
5. [x] Confirm `mdsmith check .` and the full suite stay green;
   verify race-cleanliness under `-race` (the cache is read by the
   parallel worker pool and the LSP's concurrent readers).
   `go test ./... -race` green; `go run ./cmd/mdsmith check .`
   green; `go tool golangci-lint run` clean.

## Acceptance Criteria

- [x] A target file globbed by N host-file catalogs is read and
      parsed once per `mdsmith check` run, not N times (pinned by
      a counting-FS test over a multi-host fixture).
      `TestRunCache_CrossHostCatalogReadsTargetOnce` Checks two
      host files that share one wrapped FS plus one `RunCache`;
      the counter records exactly 2 reads per shared target
      (front matter + include-cycle scan), the same as a single
      Check, instead of doubling to 4 across two hosts. The
      regression check (disabling `f.RunCache = cache`) produces
      Observed=4, which the assertion rejects.
- [x] The LSP re-reads a target after it is edited (pinned by an
      Invalidate test); no stale catalog body is served.
      `TestRunCache_InvalidateForcesReread` invalidates one of
      two cached targets and confirms only that target's counter
      bumps on the next Check; the un-invalidated sibling stays
      at its baseline.
- [x] Repo-corpus wall time improves and the repo-vs-neutral gap
      narrows further; neutral is unchanged; the check gate stays
      within budget. See task 4 above.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
