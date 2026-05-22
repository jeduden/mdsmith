---
id: 197
title: Fork goldmark — cut the per-parse allocator hot spots
status: "🔲"
model: opus
depends-on: [195, 196]
summary: >-
  After plan 195 the engine bench sits at 635 k allocs/op. The
  top five remaining allocators are all inside goldmark's parser:
  NewTextSegment, Segments.Append, NewBlockReader, NewParagraph,
  and the link-reference walker. Together they account for 55 %
  of every check's allocations. Vendor goldmark into
  internal/goldmark, swap each hot allocator for a pooled
  variant, and keep pkg/markdown's public surface unchanged so
  downstream callers do not see the fork.
---
# Fork goldmark — cut the per-parse allocator hot spots

## Goal

Drop the engine bench's allocs/op from ~635 k to ~280 k —
half the work that goldmark's parser is allocating today.
The target is the five per-paragraph, per-line, and
per-token allocators the plan-195 profile names. Each
one is shape-stable across calls. Each one is pool-able.

The fork keeps mdsmith's `pkg/markdown` API surface
identical so the LSP, the CLI, and the rule packages do
not need any change.

## Background

The plan-195 profile of `BenchmarkCheckCorpusLarge`
ranks goldmark's internals first:

| #   | Symbol                              | allocs/op | % of total |
|-----|-------------------------------------|----------:|-----------:|
| 1   | `goldmark/ast.NewTextSegment`       | 1.42 M    | 15.5 %     |
| 2   | `goldmark/text.(*Segments).Append`  | 1.26 M    | 13.8 %     |
| 3   | `goldmark/text.NewBlockReader`      | 1.24 M    | 13.6 %     |
| 4   | `goldmark/ast.NewParagraph`         | 1.09 M    | 12.0 %     |
| 5   | `goldmark/parser.newLinkLabelState` | 88 k      | 1.0 %      |

The total 55 % does not include `goldmark/ast.NewLink`,
`NewFencedCodeBlock`, `NewHeading`, and other smaller
allocators that the same patterns address.

[Plan 193](193_mds024-allocation-budget.md) set the
precedent. The Punkt segmenter sat inside an external
package whose per-token allocations dominated MDS024.
The fix forked the minimum subset into
[`internal/punkt/`](../internal/punkt/), pooled the
hot allocators, and ran an equivalence harness against
upstream. The same shape works for goldmark.

[`pkg/markdown`](../pkg/markdown/) is the public surface
the LSP and rule packages reach through. Plan 175
extracted it. The fork lives behind that surface so
callers see no API change.

## Approach

Two stages. Stage one vendors goldmark and locks in an
equivalence gate. Stage two replaces the hot allocators
one at a time, each in its own commit with its own
benchmark delta.

### Stage one: vendor + equivalence

Copy the parts of `github.com/yuin/goldmark` that
mdsmith actually uses into `internal/goldmark/`. The
import graph survey (plan-197 task 1) is the source of
truth for what to vendor — at the time of writing it is
the `ast`, `parser`, `text`, `util`, and `extension`
subtrees plus the top-level `goldmark` package.
Anything mdsmith does not reach stays out.

The fork point gets a commit-hash header per file
(matching plan 193's `internal/punkt` shape) so a
future upstream merge has a known base.

The equivalence gate runs goldmark and the fork over
the same fixture corpus and asserts byte-identical
AST output. The fixture corpus is the existing
`internal/rules/MDS*` directories plus the LSP's
`internal/lsp/testdata`. A drift fails CI.

### Stage two: replace hot allocators

Each allocator is one focused commit:

1. **`text.Segments` slice pool.** Today every
   paragraph allocates a fresh `[]Segment` and grows it
   geometrically as the parser appends. A `sync.Pool`
   of `*Segments` reuses the backing across paragraphs.
   The pool is per-`Parser`, so concurrent parses on
   different files get distinct pools (the engine's
   file-level worker pool already creates one Parser
   per file via `pkg/markdown.ParseContext`).

2. **`ast.NewTextSegment` arena.** Every text node
   allocates a `Segment{Start, Stop}`. Replace with an
   arena allocator that hands out segments from a
   pre-allocated slab. The slab resets at parser
   shutdown. Per-paragraph cost drops from
   "N allocations" to "amortised 0".

3. **`ast.NewParagraph` pool.** Same shape as the
   segment arena: each paragraph node is a uniform
   struct, pool keyed on `*Paragraph`.

4. **`text.NewBlockReader` reuse.** The parser
   allocates a fresh reader per block. The reader is
   stateful but resettable. A per-`Parser` reader
   reset between blocks eliminates the per-block
   allocation entirely.

5. **`parser.newLinkLabelState` pool.** Link-label
   parsing allocates state per `[label]`. Pool keyed
   on the state struct.

Each commit:

- Runs the equivalence harness against goldmark
  upstream. Byte-identical or the commit fails.
- Reports a `BenchmarkCheckCorpusLarge` delta. The
  alloc count must drop by at least the amount the
  profile attributed to the patched allocator.
- Lands its own unit test covering the pool's
  recycle path (e.g. "the same pointer comes back
  after Reset").

### Stage three: keep upstream tracked

The vendored copy gets a `UPSTREAM_VERSION` constant
and a `make verify-goldmark` target that downloads the
upstream tag and re-runs the equivalence harness. The
target is wired into a weekly CI job so drift between
the fork and upstream is visible. Plan 193's Punkt
fork uses the same pattern via
`internal/punkt/upstream_oracle_test.go`.

## Tasks

1. [ ] Inventory every `github.com/yuin/goldmark`
   import in the mdsmith tree. Group by package
   (ast, parser, text, util, extension). Record the
   set in this plan as the "vendor manifest".
2. [ ] Copy the vendor manifest into
   `internal/goldmark/` with a per-file commit hash
   header. Update every mdsmith import path. CI must
   stay green on the unchanged binary.
3. [ ] Add the equivalence harness at
   `internal/goldmark/equivalence_test.go`. It walks
   the MDS fixture corpus and the LSP testdata, parses
   each file through upstream and through the fork,
   and compares the AST via
   `reflect.DeepEqual` on the goldmark `ast.Node`
   tree. Byte-identical or the test fails.
4. [ ] Add a build tag `goldmark_upstream` that
   selects the upstream package for A/B comparison.
   The equivalence harness runs under both tags. The
   default build uses the fork. Same shape as
   plan 193's `mdtext_punkt_upstream`.
5. [ ] Land the `text.Segments` pool. Re-run the
   equivalence harness + the engine bench. Allocs
   must drop by ≥ 1 M objects on
   BenchmarkCheckCorpusLarge.
6. [ ] Land the `ast.NewTextSegment` arena. Allocs
   must drop by ≥ 1 M objects.
7. [ ] Land the `ast.NewParagraph` pool. Allocs must
   drop by ≥ 800 k objects.
8. [ ] Land the `text.NewBlockReader` reuse. Allocs
   must drop by ≥ 800 k objects.
9. [ ] Land the `parser.newLinkLabelState` pool.
   Allocs must drop by ≥ 50 k objects.
10. [ ] After every stage-two patch, tighten the
    engine-bench `Allocs` budget to the new measured
    value plus 5 %.
11. [ ] Add the `make verify-goldmark` target and
    wire it into a weekly CI job. The target fails
    fast if the equivalence harness drifts.
12. [ ] Update [`docs/development/index.md`][devix]
    to document the fork point, the equivalence
    contract, and the workflow for merging an
    upstream change.

[devix]: ../docs/development/index.md

## Risk

The fork carries a real maintenance burden. Every
goldmark upstream change has to be reviewed and
potentially re-applied. Mitigations:

- The `goldmark_upstream` build tag keeps the upstream
  pipeline alive for A/B testing.
- The weekly `verify-goldmark` job surfaces drift
  before it ships.
- Each pool patch is one commit with one focused
  change. A rebase against a future upstream merges
  by patch rather than by tree.

Pool aliasing is the other risk. A caller that retains
an `ast.Paragraph` past the next parse sees the same
pointer reused. The fix mirrors plan 193's Punkt
contract: the package doc comment pins
"caller-retained nodes after Parse are undefined" and
a unit test races a pooled node's content across two
parses to lock the rule in. The engine and the rule
packages all consume nodes within one `Parser` call,
so this contract holds in production today.

The equivalence harness is the second gate. A pool
patch that silently changed a field's value would fail
the `reflect.DeepEqual` AST compare.

## Acceptance Criteria

- [ ] `BenchmarkCheckCorpusLarge` allocs/op ≤ 290 000
      (the post-fork budget). Stages two through nine
      each enforce their own intermediate budget; the
      final number is the published gate.
- [ ] `mdtext.SplitSentences` and every existing
      sentence-segmenter and link-walker test stay
      byte-identical between fork and upstream.
- [ ] `internal/goldmark` ships an equivalence harness
      that runs under both `goldmark_upstream` and the
      default build, and the harness passes both.
- [ ] `make verify-goldmark` runs in CI weekly and
      fails on drift.
- [ ] Every new pool / arena has a unit test pinning
      its recycle contract.
- [ ] `mdsmith check .` passes.
- [ ] `go test ./...` and `go test -race ./...` pass.
- [ ] `go tool golangci-lint run` reports no issues.
