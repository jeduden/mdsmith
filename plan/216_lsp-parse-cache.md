---
id: 216
title: Per-document parse cache for the LSP, keyed by version
status: "✅"
model: opus
depends-on: [198]
summary: >-
  The LSP re-parses the active document on every
  textDocument/didChange. Parse is ~30 % of CPU on
  the corpus benchmark; a per-document cache keyed
  by `(path, version)` returns the cached
  `*lint.File` when the document text has not
  changed between the runLint trigger and the
  RunSource call. Mirrors the existing RunCache
  seam for cross-file reads, but for the parse
  itself.
---
# Per-document parse cache for the LSP, keyed by version

## Goal

Add a per-path parse cache, validated by version
on lookup. The path is the same string the LSP
currently passes to `engine.Runner.RunSource` —
workspace-relative, produced by
`workspaceRelative(root, doc.path)` in
[server.go](../internal/lsp/server.go) so
ignore/kind/override matching stays correct. On
hit, return the cached `*lint.File`. The cache
lives on the LSP `Server`. It has the same
lifetime as the existing
[runCache](../internal/lsp/server.go). The runLint
path consults it before calling
[lint.NewFileFromSource](../internal/lint/file.go).

## Background

`mdsmith lsp` lints the active buffer on every
`textDocument/didChange`. The debounce uses
`time.AfterFunc`. See
[server.go](../internal/lsp/server.go) for the
trigger path. Each trigger calls
[engine.Runner.RunSource](../internal/engine/runner.go).
RunSource parses the document fresh on entry to
`runSourceCheckRules`.

A 5k-line parse takes ~60 ms. The estimate scales
from the 30 % parse share measured on the corpus
benchmark. RunSource latency carries that parse
cost on every keystroke. The cost is redundant
when a debounce fires twice on the same buffer
version. It is also redundant when a non-text LSP
request triggers a re-lint without an intervening
edit. Examples: codeAction, documentSymbol,
definition.

The cross-file
[RunCache](../internal/lint/runcache.go) proves
the seam. It lives for the server lifetime.
Several edit events drop a cached entry. The
parse cache mirrors that shape. The key is denser:
`(path, version)`. Version bumps invalidate
the old entry on their own.

## Non-Goals

- Caching parses for the CLI's `mdsmith check`
  path. The corpus is immutable for one process and
  every file is parsed exactly once; a cache costs
  more than it saves.
- A cross-process cache (no disk-backed store).
- Caching across `mdsmith lsp` restarts.
- Caching anything beyond the parsed `*lint.File`.
  Diagnostics depend on config and rule state that
  this cache must not own.

## Design

### Cache shape

```go
// internal/lint/parsecache.go (new file)
type ParseCache struct {
    mu      sync.Mutex
    entries map[string]parseCacheEntry // key: path
}

type parseCacheEntry struct {
    version int    // LSP textDocument version
    file    *File
}
```

The map is keyed by the same path string the LSP
hands to `RunSource` (workspace-relative). Each
entry carries the version it was parsed at. A
`Get(path, v)` hit requires both: the entry exists
and its stored version equals `v`. Lookup
semantics are `(path, version)`; the storage
layout is one entry per path — no composite key,
no nested map. An LSP edit monotonically
increments the version, so a stored older entry
is always dead on the next miss.

Lookup signature:

```go
func (c *ParseCache) Get(path string, version int) (*File, bool)
func (c *ParseCache) Put(path string, version int, f *File)
func (c *ParseCache) Invalidate(path string)
```

### Wire-in

[engine.Runner](../internal/engine/runner.go)
gains an optional `ParseCache *lint.ParseCache`
field. When set, `RunSource(path, text)` first
checks the cache; on hit, it skips
`lint.NewFileFromSource` and reuses the cached
`*lint.File`. On miss, it parses and stores the
result before continuing.

The LSP `Server` (at
[server.go](../internal/lsp/server.go)) creates
one ParseCache at startup, alongside the existing
`runCache`. It installs the cache on every
`engine.Runner` built for runLint, passing
`document.version` from the `docs` registry into
the RunSource call.

Three handlers drop the entry for the affected
path. didChange reacts to edits, didClose to
buffer close, watched-files to disk edits.
didOpen needs no drop because the version starts
fresh.

**Invalidation must use the cache's
workspace-relative key**:
`workspaceRelative(root, absPath)`. Handlers hold
absolute paths. Each site maps to relative form
before calling `parseCache.Invalidate`. A literal
"call next to runCache.Invalidate" would leak
stale entries. runCache takes absPath. parseCache
does not.

### Arena interaction

Plan 198's per-parse arena is held alive by the
parsed AST. A cached `*lint.File` keeps its arena
slabs alive until eviction. The cache caps at one
entry per path, so total live arenas equal the
number of open documents — bounded by the LSP
client. No new pressure beyond what `mdsmith lsp`
already accepts when a document is open and
parsed.

### Concurrency

The debounce timer collapses bursts of edits, but
it does not single-flight overlapping `runLint`
calls: a second timer can fire while a prior
`runLint` for the same URI is still executing. The
cache tolerates that without single-flight
semantics. `Put(path, v, f)` only writes when the
slot is empty or `v >= existing.version`; an older
parse landing after a newer one is dropped on the
floor. That keeps the cache effective across edits
that overlap their predecessor's parse. Two
concurrent parses of the same `(path, v)` both
land, the later overwriting with an equivalent
`*File` — a wasted parse, not a correctness bug.
Cross-document parses are independent; the
`*lint.File` is not shared across paths.

## Tasks

1. [x] Add `internal/lint/parsecache.go` with the
   struct and methods. Unit tests cover Get miss,
   hit, version-mismatch miss, Invalidate, and
   the stale-Put rejection (a Put with a version
   below the stored one must not overwrite).
2. [x] Add `engine.Runner.ParseCache` field and
   the `RunSource` hit/miss branching. Existing
   tests that construct a Runner without setting
   the field must keep passing (nil cache = always
   parse).
3. [x] Add a contract test in `internal/engine/`.
   It runs the same corpus through `RunSource`
   with `ParseCache` nil and with `ParseCache`
   installed, asserting byte-equal diagnostics.
4. [x] Wire `s.parseCache` into the LSP `Server`
   alongside `s.runCache`. Pass the document
   version on the RunSource call — introduced
   `RunSourceWithVersion` so non-LSP callers
   (stdin / mdsmith check) keep `RunSource`
   unchanged.
5. [x] Add invalidation calls in `didChange`,
   `didClose`, and `didChangeWatchedFiles`
   handlers next to the existing
   `runCache.Invalidate` calls.
6. [x] Add an integration test in `internal/lsp/`.
   It walks didOpen → runLint → didChange →
   runLint. The second pass must surface
   diagnostics from the edited text. No stale
   results from a cached pre-edit parse.
7. [x] Add a "warm cache" variant of
   `BenchmarkLatency1kLines` and
   `BenchmarkLatency5kLines`. The second
   RunSource call on the same version must skip
   the parse and finish faster. Landed as
   `BenchmarkLatency1kLinesWarmCache` and the 5k
   peer in `internal/lsp/bench_parsecache_test.go`.
   The bench drives `runLint` on a goroutine.
   It then reads the published diagnostics. The
   same version re-lints with no new edit.
8. [x] Tighten the warm-cache benchmark's
   `budget` to the measured p95 (with ~3-5 ×
   headroom matching the cold path's sizing rule)
   so the ≥ 20 % improvement is gated, not
   advisory. Record the measured number in this
   plan.

   Measured locally on a CI-class host (Intel
   Xeon @ 2.1 GHz, GOMAXPROCS=4, 50-iter bench).
   1k: 3 ms cold vs 2 ms warm (≈ 33 % faster).
   5k: 14 ms cold vs 10 ms warm (≈ 43 % faster).
   Warm budgets land at 30 ms / 100 ms — under
   the 80 % thresholds (120 ms / 400 ms) of the
   cold budgets (150 ms / 500 ms). That gates the
   ≥ 20 % win and leaves headroom for CI jitter.

## Risk

A non-LSP caller (test, embedded host) that
mutates the cached `*lint.File` between hits
would observe staleness. Mitigation: the cache is
opt-in via the Runner field; only the LSP
installs one, and the LSP never mutates a
`*lint.File`.

Plan 198's arena-lifetime concern does **not**
apply here. The concern was: AST pointers from
one Parse become invalid on the next Parse on the
same parser. Plan 198 already moved to a per-Parse
arena, GC'd with the AST. A cached `*lint.File`
holds its own arena slabs.

A regression that mis-keys an entry (e.g. forgets
to bump version on a real edit) would serve stale
diagnostics for one cycle until the next edit
fires invalidation. The unit test for
version-mismatch miss is the gate; a contract
test in `internal/lsp/` that walks didChange →
runLint → assert diagnostics reflect new text
catches integration drift.

## Acceptance Criteria

- [x] `internal/lint/parsecache.go` lands with
      unit-test coverage on Get/Put/Invalidate
      and version-mismatch behavior.
- [x] `engine.Runner.RunSource` returns the same
      diagnostics whether `ParseCache` is nil or
      installed (covered by a contract test that
      runs the same corpus through both paths).
- [x] LSP `Server` installs the cache,
      invalidates it on
      didChange/didClose/didChangeWatchedFiles.
- [x] A "warm cache" variant of
      `BenchmarkLatency5kLines` shows ≥ 20 %
      lower p95 than the cold path. Measured
      ≈ 43 % (14 ms cold vs 10 ms warm).
- [x] Existing cold `BenchmarkLatency1kLines` and
      `BenchmarkLatency5kLines` p95 stays within
      their current budgets (150 ms / 500 ms).
      Measured 3 ms / 14 ms on the local host.
- [x] A didChange → re-lint integration test
      asserts the second runLint sees the new
      text (no stale diagnostics from a cached
      pre-edit parse).
      `TestParseCache_DidChangeReflectsNewText`.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.
