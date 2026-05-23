---
id: 198
title: Fork goldmark with a per-parse arena for the four structural allocators
status: "🔲"
model: opus
depends-on: [197]
summary: >-
  Plan 197 PoC measured a 12.8 % allocs/op cut from
  one tactical change to goldmark's link-ref
  transformer (BlockReader reuse, the only tactical
  target in the top-5 hot allocators). The remaining
  four — NewTextSegment, NewParagraph, Segments.Append
  backing arrays, and FindClosure's NewSegments — are
  all structural: pointers escape to AST or back-array
  growth fires from inside the parser. A per-parse
  arena threaded through parser.Parser is the only
  shape that absorbs all four. Combined ceiling is
  ~41 % of corpus allocations. This plan vendors the
  goldmark subset, implements the arena, gates it
  with a build-tag A/B + equivalence harness, and
  ships behind `pkg/markdown`.
---
# Fork goldmark with a per-parse arena for the four structural allocators

## Goal

Land a goldmark fork at `pkg/goldmark/`. Its
`parser.Parser` carries a per-parse arena. The arena
absorbs four structural allocators from
[plan 197's matrix](197_fork-goldmark-for-allocs.md#review-matrix):
`ast.NewTextSegment`, `ast.NewParagraph`,
`text.(*Segments).Append` backing arrays, and
`text.FindClosure`'s `NewSegments`. Combined ceiling
is ~41 % of allocations. Plan 197's
`linkrefparagraph` stays as a prior win.

## Background

[Plan 197](197_fork-goldmark-for-allocs.md) shipped
the matrix's one tactical PoC: a per-parser
`linkrefparagraph.Transformer`. It reuses one
`text.BlockReader` across all paragraphs in a parse.
Measured savings on `BenchmarkCheckCorpusLarge
-benchtime=10x` were 634 k → 553 k allocs/op
(−12.8 %), inside 6 % of the predicted 13.6 %. Wall
time dropped from 264 ms p95 to 247 ms.

The other four hot allocators are structural. They
escape to the AST or grow per-block backing arrays.
A pool-in-place will not work. Each allocation's
lifetime is "until the AST consumer is done with the
document". mdsmith consumes AST inside one Parse
call (CLAUDE.md). That makes a per-parse arena the
right shape.

The arena's API contract:

- One `arena.Arena` lives on the `parser.Parser` for
  the duration of one `Parse(reader, opts...)` call.
- Allocators inside `pkg/goldmark/ast/` and
  `pkg/goldmark/text/` route through the arena
  instead of `new(T)`.
- `Parse` returns; `arena.Reset()` is deferred so
  the slab is reusable on the next call.
- AST node pointers returned from `Parse` remain
  valid until the *next* call to `Parse` on the
  same parser. mdsmith's consumers (rule packages,
  LSP server) already consume AST inside one Parse;
  this contract is documented in
  [`pkg/markdown`](../pkg/markdown/).

## Approach

Four stages.

### Stage one — vendor

Copy goldmark@v1.8.2 to `pkg/goldmark/`. Keep
the package layout (`ast/`, `text/`, `parser/`,
`util/`). Rewrite imports. Plan 197's
`linkrefparagraph` folds into the vendored `parser/`
as the default link-ref transformer. Keep upstream
tests at their original paths.

### Stage two — add the arena

`pkg/goldmark/arena/arena.go` exposes a slab
allocator. Typed helpers: `Text()`, `Paragraph()`,
`Segments(cap)`. `Reset()` discards live pointers
and resets cursors. Constructors in vendored `ast/`
and `text/` accept a nil-safe `*arena.Arena`. The
`parser.Parser` carries one arena and defers Reset.

### Stage three — equivalence harness

`pkg/goldmark/equivalence_test.go` runs every
upstream test through the fork. It diffs AST shape
and rendered HTML. The harness gates every later
arena change.

### Stage four — measure and gate

Re-run `BenchmarkCheckCorpusLarge -benchtime=10x`
with the arena landed. Expected target: ≥ 35 % cut
from the post-plan-197 baseline (553 k → ≤ 360 k
allocs/op). Wall time ≤ post-plan-197 baseline.

A build-tag A/B (`-tags goldmark_upstream`) lets CI
diff the two paths on the same source until the fork
is the only path.

## Tasks

1. [x] Vendor goldmark@v1.8.2 under
   `pkg/goldmark/`. Rewrite imports. `go build
   ./...` and `go test ./pkg/goldmark/...` stay green
   with the fork as a drop-in.  (Done in the current
   PR — pkg/goldmark/ is wired via `replace
   github.com/yuin/goldmark => ./pkg/goldmark` in
   the root go.mod.  Because pkg/goldmark is a
   nested module, fork-specific tests run via an
   explicit `go test ./...` inside pkg/goldmark/ — see
   the CI workflow.)
2. [x] Move plan 197's `linkrefparagraph` into the
   vendored `parser/` package as the default link-ref
   transformer. Delete the old standalone package.
   (Done in the current PR — the transformer is at
   pkg/goldmark/parser/link_ref.go with the
   BlockReader-reuse + Reset semantics, and the
   standalone linkrefparagraph package is removed.)
3. [ ] Add `pkg/goldmark/arena/` with the typed
   slab allocator. Reset is idempotent.
4. [ ] Thread the arena through `ast.NewText`,
   `ast.NewParagraph`, `text.NewSegments`, and
   `text.(*Segments).Append`'s backing array
   allocation.
5. [ ] Wire `parser.Parser` to own the arena and
   defer Reset on `Parse` return.
6. [ ] Add the equivalence harness — every upstream
   goldmark test runs against the forked parser and
   diffs AST + HTML.
7. [ ] Add the build-tag A/B path so CI can lint the
   same source through both.
8. [ ] Re-run `BenchmarkCheckCorpusLarge` and record
   results in this plan.
9. [ ] Update [docs/development/index.md](../docs/development/index.md)
   to point at the fork as the canonical parser.

## Risk

The arena couples AST lifetime to the next Parse
call on the same parser. mdsmith's `parserPool` in
[pkg/markdown/parser.go](../pkg/markdown/parser.go)
returns parsers between parses, so two consecutive
calls on the same goroutine may share a pool slot
and the second Reset invalidates the first AST.
Mitigation: an audit pass over every consumer, and
an opt-in `parser.WithNoArena()` for callers that
need long-lived AST.

The fork diverges from upstream.

Mitigation: the equivalence harness gates every
change.  A quarterly upstream-merge task keeps drift
visible.  This task lives in `plan/` alongside this
plan rather than in `docs/development/secret-rotations.md`
— that file is scoped to credential rotation only and
is not the right home for fork-maintenance cadence.

## Acceptance Criteria

- [ ] `pkg/goldmark/` is the canonical parser
      and `pkg/markdown` imports only from it.
- [ ] `BenchmarkCheckCorpusLarge -benchtime=10x`
      median allocs/op ≤ 360 k (≥ 35 % cut from
      553 k post-plan-197 baseline).
- [ ] `BenchmarkCheckCorpusLarge` p95 wall time ≤
      247 ms (post-plan-197 baseline).
- [ ] Equivalence harness passes — every upstream
      goldmark test runs through the fork with
      identical AST + HTML.
- [ ] `go test ./...` and `go test -race ./...`
      green.
- [ ] `mdsmith check .` green.
- [ ] `go tool golangci-lint run` reports no issues.
