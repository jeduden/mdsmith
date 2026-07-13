---
id: 2607121915
title: >-
  Resolve the internal/linkgraph purity-contract
  mismatch for wikilink resolution
status: "🔳"
model: sonnet
summary: >-
  go.md documents internal/linkgraph as pure (no file
  reads, no workspace walks), but NewWikilinkIndex and
  ResolveWikiLink both call fs.WalkDir. Flagged by the
  2026-07-12 audit.
---
# Resolve the linkgraph purity-contract mismatch

## Goal

Make [go.md](../docs/development/architecture/go.md) match
what `internal/linkgraph` actually does. Callers should be
able to trust the documented concurrency contract.

## Background

The 2026-07-12 audit (see
[the audit log](../docs/development/architecture-audit.md))
found that
[go.md](../docs/development/architecture/go.md) states:

> `internal/linkgraph` — canonical Markdown link /
> directive / reference extractor; ... Pure (no file
> reads, no workspace walks); callers can fan it out
> across goroutines.

But `NewWikilinkIndex` and `ResolveWikiLink` in
[wikilinks.go](../internal/linkgraph/wikilinks.go) both
call `fs.WalkDir`. They resolve `[[wikilink]]` targets by
walking the workspace tree. This is the only file in the
package that touches the filesystem. Every other
extractor (`ExtractWikiLinks` and friends) is genuinely
pure, matching the doc.

Two call sites depend on the current behavior:

- [backlinks.go](../cmd/mdsmith/backlinks.go)
- [crossfilereferenceintegrity/rule.go](
  ../internal/rules/crossfilereferenceintegrity/rule.go)

`internal/index` already owns workspace-wide graph
building per go.md's package table. That makes it the
natural home for a workspace walk, if the walk moves
rather than the doc.

The audit found one more thing. `NewWikilinkIndex` and
`ResolveWikiLink` each reimplement the same stem/name
walk-and-match algorithm. They share only
`sortByDepthThenName`, `skipHeavyDirs`, and
`wikilinkSearchKey`. Fixing the purity question likely
means rewriting one in terms of the other anyway, so
resolve both in the same pass.

## Tasks

1. Decide the resolution: either (a) move the
   `fs.WalkDir`-based index build into `internal/index`
   and have `internal/linkgraph` expose only extraction +
   resolution-against-precomputed-data, or (b) narrow
   go.md's purity claim to scope it correctly to the
   per-file extraction functions and document
   `WikilinkIndex`/`ResolveWikiLink` as the sanctioned
   workspace-walking exception.
2. If (a): implement `ResolveWikiLink` as
   `NewWikilinkIndex(root).Resolve(target)` so the walk
   logic exists in one place, then move the index-building
   code and update both call sites.
3. If (b): update go.md's `internal/linkgraph` entry with
   the precise scope of the purity guarantee.
4. `go build ./...` passes.
5. `go test ./internal/linkgraph/... ./cmd/mdsmith/...
   ./internal/rules/crossfilereferenceintegrity/...`
   passes.

## Acceptance Criteria

- [ ] go.md's description of `internal/linkgraph` matches
      the package's actual behavior.
- [ ] `NewWikilinkIndex` and `ResolveWikiLink` no longer
      maintain two independent copies of the walk/match
      algorithm.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
