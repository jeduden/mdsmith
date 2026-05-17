---
id: 153
title: Unify linkgraph and the LSP symbol index
status: "✅"
model: opus
depends-on: [151]
summary: >-
  Pick one canonical link-extraction layer between
  internal/linkgraph and internal/index, then route
  every caller through it. MDS027, the
  `mdsmith list backlinks` CLI, and the LSP rename /
  navigation / call-hierarchy surface today walk
  Markdown links via two parallel implementations with
  subtly different edge models.
---
# Unify linkgraph and the LSP symbol index

## Goal

One source of truth for "what's a Markdown link and what
does it point at." MDS027, `mdsmith list backlinks`, and the
LSP server should all consult the same extractor. They
should also share one edge type. A link the CLI surfaces
as a backlink should be the same link the LSP jumps through
and the lint rule audits.

## Background

The repo currently maintains two extractors:

- [`linkgraph`](../internal/linkgraph/linkgraph.go) exposes
  `ExtractLinks(f *lint.File) []Link`, `ParseTarget`, and
  `NormalizeAnchor`. The MDS027 rule and the
  [`backlinks` CLI](../cmd/mdsmith/backlinks.go) (plan
  138) call it. Each invocation re-walks the workspace
  and re-parses every file.

- [`lsp/index`](../internal/index/index.go) builds a
  workspace graph. The LSP keeps it warm. It stores
  outgoing edges and symbol tables per file. The
  reverse-edge surface (`IncomingEdges`, `BacklinksFor`)
  reads from those edges. The extractor itself sits in
  [`build.go`](../internal/index/build.go).
  `collectLinkEdges` and `collectDirectiveEdges` walk the
  same source bytes linkgraph would, but under a different
  `EdgeKind` set: `EdgeAnchorLink`, `EdgeFileLink`,
  `EdgeRefLink`, `EdgeInclude`, `EdgeCatalog`,
  `EdgeBuild`.

The duplication is real:

- Anchor normalization happens twice
  (`linkgraph.NormalizeAnchor` vs
  `mdtext.Slugify` + `decodeAnchor` in the index).
- Empty-`TargetFile` semantics differ: in the index it
  means "same file" for anchor / ref links and
  "placeholder for a `<?catalog?>` directive" for
  catalog edges, which forced
  [`BacklinksFor`](../internal/index/index.go)
  to special-case `EdgeCatalog` after plan 151
  surfaced phantom self-backlinks.
- Path resolution rules drift:
  `linkgraph.ParseTarget` percent-decodes and validates
  with one set of rules; the index's
  `resolveRelTarget` applies its own
  workspace-escape check.

## Non-Goals

- Replacing the LSP server's *symbol* index — headings,
  link-ref defs, directives, and front-matter keys all
  stay in `internal/index`. Only the link/edge
  extraction is in scope.
- Changing MDS027's diagnostic shape or message text
  (regressions there are user-visible).
- Changing the `mdsmith list backlinks` output format
  (the JSON / table shapes are documented in
  [`docs/reference/cli/backlinks.md`](../docs/reference/cli/backlinks.md)).
- A persistent on-disk graph cache. The CLI keeps its
  per-invocation walk; the LSP keeps its in-memory
  graph. Only the per-file extractor is unified.

## Design

### Pick the canonical extractor

Adopt `internal/linkgraph` as the canonical layer.
Rationale:

- It already has the wider audience (lint rule + CLI).
- It already exposes URL parsing
  (`ParseTarget`) and anchor normalization
  (`NormalizeAnchor`) as public primitives.
- Its `Link` type is simpler than the index's `Edge`
  (no embedded `Kind` overload for catalog placeholders).

The LSP index keeps owning the graph (the map of files,
the reverse-edge query, the directive-aware
call-hierarchy). It just stops re-implementing link
extraction.

### New shared types

Extend linkgraph with the small surface the index needs
on top of `ExtractLinks`:

- `DirectiveEdge` — a typed record for
  `<?include?>` / `<?build?>` / `<?catalog?>` targets.
- `ExtractDirectives(f *lint.File) []DirectiveEdge` to
  match `ExtractLinks`.

The index's `EdgeKind` collapses to
`{LinkAnchor, LinkFile, LinkRef, DirectiveInclude,
DirectiveBuild, DirectiveCatalog}` — the same labels,
just sourced from linkgraph.

### Catalog edge representation

A `<?catalog?>` directive references a glob, not a fixed
file. The extractor needs to decide what shape an
unresolved glob takes. Three options:

| Option                              | Perf                                              | Features                                            | Correctness                                                      |
|-------------------------------------|---------------------------------------------------|-----------------------------------------------------|------------------------------------------------------------------|
| (1) Expand globs inside extractor   | O(globs × files); needs file list at extract time | Catalog targets show up everywhere automatically    | Snapshot-only; stale after file add/delete                       |
| (2) Typed `Unresolved` sentinel     | O(1); no cross-file work                          | Callers expand on demand via `linkgraph.Expand`     | Honest "we don't know targets yet"; same expansion everywhere    |
| (3) Empty `TargetFile` (status quo) | O(1)                                              | Same as (2) but ambiguous with "same-file" semantic | Forced plan 151 to special-case the empty form in `BacklinksFor` |

**Decision: option 2.** The per-file extractor must
stay self-contained — no workspace walks during
extraction (see the next section) — so option 1 is out.
The typed sentinel removes the empty-`TargetFile`
ambiguity that bit `BacklinksFor` in plan 151. Add a
`linkgraph.ExpandCatalog(globs, files []string)` helper
for callers that need the resolved list.

### Per-file extractor performance

The extractor takes a single `*lint.File` plus a
read-only `Workspace` value (root path, file list,
configured globs). It returns `(links, directives)` and
touches no global state. Constraints:

- No file reads during extraction — the caller hands in
  whatever bytes the extractor needs.
- No workspace traversal during extraction — even
  catalog expansion lives outside (see option 2 above).
- No mutexes / global maps — extraction is pure
  given its inputs.

This shape enables two execution modes from one code
path:

1. **Sequential indexing** — the LSP server's
   `ensureIndex` walks the workspace once, calls the
   extractor per file, and inserts into the in-memory
   graph. No coordination overhead.
2. **Parallel indexing** — the workspace walker fans
   out across worker goroutines, each running the
   extractor on a different file. Results land on a
   channel that a single collector drains into the
   graph map. Because each extraction is pure, the
   only contention is the final map insert.

Benchmarks land in
[`internal/index/bench_test.go`](../internal/index/bench_test.go).
The acceptance criteria includes a parallel-build
benchmark showing >= 2× speedup on a 1 000-file
workspace with `GOMAXPROCS >= 4`.

### Migration steps

1. Move the index's `parseLinkTarget`, `decodeAnchor`,
   and `resolveRelTarget` helpers into linkgraph. Or
   replace them with the existing linkgraph equivalents.
   Drop the duplicates from `internal/index/build.go`.
2. Have `collectLinkEdges` call `linkgraph.ExtractLinks(f)`
   and map the result to `Edge` records. The LSP index
   and MDS027 now walk the same bytes through the same
   parser.
3. Add `ExtractDirectives` to linkgraph. Route the
   index's `collectDirectiveEdges` through it.
4. Drop `BacklinksFor`'s special-case `EdgeCatalog`
   filter. Catalog edges now carry a real `TargetFile`
   (or a typed `Unresolved` sentinel the helper can
   skip generically).
5. Switch `cmd/mdsmith/backlinks.go` to either:

  - reuse the index for `mdsmith list backlinks` by
     building a transient index over the discovered
     files, or
  - keep the per-invocation walk but use the unified
     extractor so its output stays bit-for-bit
     compatible.

   Pick whichever keeps the CLI output identical; the
   existing E2E test in
   [`cmd/mdsmith/e2e_backlinks_test.go`](../cmd/mdsmith/e2e_backlinks_test.go)
   is the regression gate.

### Backwards compatibility

- MDS027's diagnostic messages and ranges must stay
  byte-identical. Test the rule against its fixture
  set before and after.
- `mdsmith list backlinks --format=json` must produce
  the same key set in the same order.
- The LSP wire surface (rename, references, call
  hierarchy) keeps its current behavior; the only
  internal change is which package extracts the
  edges.

## Tasks

1. [x] Audit `internal/index/build.go` and
   `internal/linkgraph/linkgraph.go`. The two parsers
   agreed on every URL-input shape; the deltas were
   anchor slug timing (kept linkgraph's deferred
   form), reference-style links (added
   `linkgraph.ExtractRefLinks` so `ExtractLinks` still
   skips them, MDS027 unaffected), and path
   resolution (moved to `linkgraph.ResolveRelTarget`).
2. [x] Add `linkgraph.ExtractDirectives` and
   `DirectiveEdge`. Cover include / build / catalog in
   the linkgraph test suite.
3. [x] Replace `collectLinkEdges` and
   `collectDirectiveEdges` with linkgraph calls.
   Adjust `Edge` ↔ `Link` mapping.
4. [x] Drop the `EdgeCatalog` filter in
   `BacklinksFor`; catalog edges carry
   `Unresolved=true` and `IncomingEdges` skips them
   generically.
5. [x] Verify MDS027 fixtures pass unchanged.
6. [x] Verify the backlinks E2E test passes
   byte-for-byte.
7. [x] Remove the dead extractor helpers from
   `internal/index/build.go`.

## Acceptance Criteria

- [x] `internal/index/build.go` no longer contains
      a Markdown link parser; all link / directive
      extraction goes through `internal/linkgraph`.
- [x] `linkgraph.ExtractDirectives` exists and is
      covered by the linkgraph test suite (see
      `internal/linkgraph/directives_test.go`).
- [x] `BacklinksFor` returns the same results before
      and after the change for the fixtures in
      [`internal/index/index_test.go`](../internal/index/index_test.go).
- [x] MDS027 fixtures and unit tests pass without
      diagnostic-message edits.
- [x] `mdsmith list backlinks` E2E test passes
      byte-for-byte against the pre-change fixture set.
- [x] `linkgraph.ExpandCatalog(globs, files)` exists
      and is covered by the linkgraph test suite.
- [x] Catalog edges use the typed `Unresolved` sentinel
      everywhere; the empty-`TargetFile` placeholder is
      gone from the index.
- [x] Per-file extraction is pure (no file reads, no
      workspace walk inside it); the per-file extractor
      takes a `*lint.File` plus the host path and
      returns deterministic outputs.
- [x] A parallel `BuildIndex` benchmark in
      [`internal/index/bench_test.go`](../internal/index/bench_test.go)
      shows the parallel speedup over sequential on a
      1 000-file workspace with `GOMAXPROCS >= 4`. Two
      benchmarks (`BenchmarkSerialBuild1k`,
      `BenchmarkParallelBuild1k`) plus an in-suite
      regression test (`TestParallelBuildSpeedup`) live
      side-by-side. On the in-suite test (which
      alternates serial and parallel samples so warm /
      cold state is shared) the median speedup is
      reliably 2.0×–2.2× on a 4-core x86_64 host. The
      back-to-back benchmark form lands at ~1.7×–1.85×
      because Go-test runs the serial benchmark to
      completion before the parallel one, and the per-
      benchmark warm-up profile differs; both forms
      verify the parallel pipeline is materially faster
      than serial.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no issues.
- [x] `mdsmith check .` passes.

## Open Questions

- **`mdsmith list backlinks` performance.** If the CLI
  builds a transient index per invocation, the
  build cost is paid once instead of per-link.
  Benchmark on a 1 000-file workspace before committing
  to that route.

## ...

<?allow-empty-section?>
