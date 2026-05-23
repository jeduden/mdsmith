---
id: 197
title: PoC — review goldmark's allocation architecture, then pool the best lever
status: "✅"
model: opus
depends-on: [195]
summary: >-
  Before plan 198 commits to a full goldmark fork, do an
  architecture review of why goldmark allocates the way it does
  — then PoC the single change the review identifies as the
  biggest lever. The plan-195 profile names five hot allocators
  but does not say which are tactical (pool the existing type)
  versus structural (the parser allocates a fresh X per Y where
  Y could share one X). NewBlockReader is the canonical example
  — its type already has Reset(), the parser just chooses to
  allocate fresh per paragraph. The review separates the two
  kinds; the PoC validates the largest fix with a measured
  benchmark delta and a pass/fail decision on plan 198.
---
# PoC — review goldmark's allocation architecture, then pool the best lever

## Goal

Decide whether to write plan 198 (full fork) based on
two measured artifacts:

1. An architecture review of goldmark's per-parse
   allocations, categorised as tactical (pool the
   existing type) versus structural (the parser
   allocates a fresh X per Y where Y could share one).
2. A PoC of the single biggest lever the review names,
   with side-by-side benchmark numbers against the
   pre-PoC baseline.

The deliverable is a numbered recommendation, not a
shipped fork. The PoC branch is throwaway. The plan
either schedules plan 198 with the review's full target
list and the PoC's measured savings as justification,
or closes 197 as ⛔ with a Results section that explains
why the leverage is not there.

## Background

[Plan 195](195_per-rule-alloc-budget.md)'s profile of
`BenchmarkCheckCorpusLarge` attributes 55 % of every
check's allocations to five goldmark allocators:

| #   | Symbol                              | allocs/op | % of total |
|-----|-------------------------------------|----------:|-----------:|
| 1   | `goldmark/ast.NewTextSegment`       | 1.42 M    | 15.5 %     |
| 2   | `goldmark/text.(*Segments).Append`  | 1.26 M    | 13.8 %     |
| 3   | `goldmark/text.NewBlockReader`      | 1.24 M    | 13.6 %     |
| 4   | `goldmark/ast.NewParagraph`         | 1.09 M    | 12.0 %     |
| 5   | `goldmark/parser.newLinkLabelState` | 88 k      | 1.0 %      |

The counts are dramatic. The profile does not
explain *why* each one fires that often. Some
examples hint at structural issues, not just "types
with no pool":

- `text.NewBlockReader` is allocated once per
  paragraph by the link-reference transformer
  (`parser/link_ref.go:18`). The `blockReader` type
  already has `Reset(segments)` — the parser just
  chooses to allocate a fresh one per call. ~45 k
  paragraphs per bench iteration × 10 iterations is
  the bulk of the 1.24 M count.
- `ast.NewTextSegment` and `text.Segments.Append`
  scale with the number of inline text segments per
  paragraph. The segments list lives on the paragraph
  node, so the lifecycle is bounded by the parse — a
  per-parse arena would collapse the per-segment
  allocs to one slab.
- `ast.NewParagraph` is one alloc per paragraph node.
  Every AST node in goldmark is heap-allocated and
  reached via interface; an arena allocator would
  collapse the node allocs to one slab too.

Three of the five top allocators look structural,
not tactical. A naive "add a sync.Pool" fix to each
one would land savings. But the structural shape may
unlock a multiple. A single arena that retires on
parse end could eat the cost of four pools and a
goroutine of pool bookkeeping.

[Plan 193](193_mds024-allocation-budget.md)'s Punkt
fork is the precedent for "fork a parser, pool its
allocators, gate with an equivalence harness". That
work was tactical because the segmenter's allocator
shape was already the right one — the fork added
pools without restructuring. Goldmark may need both.

[`pkg/markdown`](../pkg/markdown/) is the public
surface every change lives behind. Plan 175 extracted
it. The PoC patches the parser internals without
moving the public API.

## Approach

Three stages. Stage one is the review. Stage two is
the PoC informed by it. Stage three is the decision.

### Stage one — architecture review

For each allocation site in the plan-195 profile,
record lifecycle, reuse barrier, category, estimated
saving, and risk. The matrix lives below. The review
also answers three cross-cutting questions:

- Could one per-parse arena replace four of the five
  hot allocators?
- Does the link-ref transformer's BlockReader persist
  any state?
- What structural opportunities did the profile miss?

### Stage two — PoC the biggest lever

Rank the matrix by estimated saving. Implement the
top target only, on a throwaway branch. Vendor the
minimum goldmark subset. No build tag, no equivalence
harness, no A/B path — those are plan 198 costs.

### Stage three — measure and decide

Side-by-side against pre-PoC main:
`BenchmarkCheckCorpusLarge -benchtime=10x` for allocs
and p95, `go test ./...` for behavioural equivalence.

- **Pass** = alloc savings within 10 % of the
  prediction AND wall time ≤ baseline.
- **Fail** = either condition false. Explain which.

Pass writes plan 198 with the matrix as its work plan.
Fail closes 197 as ⛔.

## Tasks

1. [x] Read `goldmark/parser/parser.go`,
   `goldmark/parser/link_ref.go`, `goldmark/text/reader.go`,
   `goldmark/text/segments.go`, `goldmark/ast/*.go`,
   and any extension under `goldmark/extension/` that
   the engine bench reaches. Note the lifecycle and
   reuse-barrier for every allocation site the
   plan-195 profile names.
2. [x] Build the review matrix below. One row per
   allocator. Columns: lifecycle, reuse barrier,
   category, estimated saving, risk.
3. [x] Answer the cross-cutting questions in a short
   "review findings" subsection.
4. [x] Rank by estimated saving. Pick the highest.
   Document the choice and the runner-up so the
   alternative is on record.
5. [x] Vendor the minimum goldmark subset the change
   touches into `pkg/goldmark/linkrefparagraph/`.
   `go build ./...` and `go test ./...` stay green.
6. [x] Implement the chosen change (per-parser
   transformer instance carrying a reusable
   BlockReader, Reset on every paragraph). Run
   `go test ./...` again. Any failure stops the PoC
   and gets recorded in Results.
7. [x] Capture the side-by-side bench numbers (allocs
   and p95) against the pre-PoC baseline. Same
   machine, same minute.
8. [x] Fill in this plan's Results section with the
   review prediction and the PoC measured numbers.
9. [x] On pass, write plan 198 — the full fork —
   carrying the review matrix forward as its work
   plan and the PoC numbers as the justification.
   See [plan 198](198_goldmark-arena-fork.md).
10. [ ] On fail, close 197 as ⛔ and write the
    rationale into the Results section. *Not applicable
    — PoC passed.*

## Review matrix

Confirmed in `goldmark@v1.8.2`. Profile percentages are
the plan-195 share of total allocations.

| Allocator                                                         | Lifecycle                                   | Reuse barrier                                                                                                                                                                                                                          | Category       | Est. saving           | Risk                                                                                                                                                                    |
|-------------------------------------------------------------------|---------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ast.NewTextSegment` (`inline.go:191`)                            | per inline text run; escapes to AST         | `*Text` is `AppendChild`-ed to AST and lives through the consumer pass (parser.go:1271, link.go:455, code_span.go:35).                                                                                                                 | **Structural** | 15.5% — arena only    | Arena requires consumers to consume AST inside the Parse-bounded window. Long-lived holders would not be safe.                                                          |
| `text.(*Segments).Append` (`segment.go:178`)                      | per block; `[]Segment` backing array        | Growth is `append`-driven on `BaseBlock.lines.values`. Each block owns its own slice.                                                                                                                                                  | **Structural** | 13.8% — arena only    | Single shared scratch hard without an arena. Pairs naturally with the AST-arena change.                                                                                 |
| `text.NewBlockReader` (`reader.go:322` @ `parser/link_ref.go:18`) | per paragraph; lone hot call site           | None — type has `Reset(*Segments)` (reader.go:351). Parser's main inline pass (`parser.go:902` + `parser.go:1165`) already runs **one** shared blockReader with Reset across all blocks. The link-ref transformer is the lone holdout. | **Tactical**   | 13.6% — pool/share    | Singleton transformer is shared across parser instances; mdsmith's `parserPool` hands one parser per goroutine, so a per-parser transformer instance is goroutine-safe. |
| `ast.NewParagraph` (`block.go:191`)                               | per paragraph; escapes to AST               | `*Paragraph` is `AppendChild`-ed into the document tree (paragraph.go:29, setext_headings.go:90).                                                                                                                                      | **Structural** | 12.0% — arena only    | Same AST-lifetime constraint as NewTextSegment. Mid-parse `RemoveChild` (paragraph.go:60) complicates a per-type pool.                                                  |
| `parser.newLinkLabelState` (`link.go:30`)                         | per `[` during inline pass; does NOT escape | Created at link.go:238, removed at link.go:454 before Parse returns. List nodes are torn down inside the same inline pass.                                                                                                             | **Tactical**   | 1.0% — pool/free-list | Lowest risk, smallest payoff.                                                                                                                                           |

## Review findings

### Per-parse arena vs four of five hot allocators

`NewTextSegment` + `NewParagraph` + `Segments.Append`
(backing-array growth) sum to **41.3 %** of corpus
allocs. All three are bounded by Parse in mdsmith's
contract (CLAUDE.md: "consumes AST inside one Parse").
A per-parse arena retiring on `Parse` return could
replace all three. Three files deep — `ast/` and
`text/` both vendored. Plan 198's territory.

### `NewBlockReader` reuse barrier

None. `blockReader` holds only `source`, `segments`,
`pos`, `line`, `head`, `last`, `lineOffset` — all
per-paragraph, all wiped by `Reset(segments)`
(reader.go:351). `parser.go:902` already shares one
blockReader across every block in the inline pass.
The link-ref transformer is the lone holdout. **One
shared instance per transformer would cover every
paragraph.** The only API gap: `blockReader.source`
has no setter, so cross-Parse source change forces a
re-allocation (still ≪ per-paragraph).

### Opportunities the profile may have missed

- `text.FindClosure` calls `NewSegments` (reader.go:668,
  689) per link scan. Some of the `Segments.Append`
  13.8 % is FindClosure's result `*Segments`, not the
  paragraph's `lines`. An arena serves both sites.
- `reader.peekedLine` invalidation (reader.go:201)
  allocates new line slices on Advance. Out of scope.

## Ranking

| Rank | Allocator                         | Est. saving | Tractability                                                  |
|------|-----------------------------------|-------------|---------------------------------------------------------------|
| 1    | **NewBlockReader at link_ref.go** | 13.6 %      | High — Reset exists, parser-internal precedent, no AST escape |
| 2    | NewTextSegment                    | 15.5 %      | Low — requires arena fork                                     |
| 3    | Segments.Append (backing array)   | 13.8 %      | Low — couples with arena                                      |
| 4    | NewParagraph                      | 12.0 %      | Low — requires arena                                          |
| 5    | newLinkLabelState                 | 1.0 %       | High but payoff below pool overhead                           |

**PoC target: NewBlockReader at `parser/link_ref.go:18`.**
The change is tactical and isolated, closing a
consistency gap within goldmark itself — the
inline-pass code already shares one blockReader the
same way. Wall time should not regress; Reset is
cheaper than allocate-and-GC, and a transformer field
avoids any sync.Pool overhead.

**Runner-up: per-parse arena over NewTextSegment +
NewParagraph + Segments.Append.** Combined ceiling
41.3 %. Plan 198 picks this up on a PoC pass; on fail
the arena becomes plan 197's actual deliverable.

## Risk

The review covers only allocators the plan-195
profile already named. Mitigation: the third
cross-cutting question explicitly looks beyond the
top-5.

The PoC scope is one change. The Results section
names what's left for plan 198 to weigh.

Pool aliasing is the standard plan-193 risk. mdsmith
rules consume AST inside one Parse call, so the "do
not retain past Parse" contract holds today. The
PoC's chosen change inherits that contract.

## Results

**Verdict: PASS.** PoC numbers below were captured on
the same machine in the same minute, three runs each
of `BenchmarkCheckCorpusLarge -benchtime=10x -count=3
-benchmem`.

Baseline (origin/main `cf363f5` — plan 195's last merged commit):

| Metric        | Run 1   | Run 2   | Run 3   | Median  |
|---------------|--------:|--------:|--------:|--------:|
| allocs/op     | 634,729 | 634,459 | 634,368 | 634,459 |
| p95 wall (ms) | 316     | 249     | 264     | 264     |
| bytes/op      | 201 MB  | 201 MB  | 201 MB  | 201 MB  |

PoC (per-parser transformer with reusable BlockReader):

| Metric        | Run 1   | Run 2   | Run 3   | Median  |
|---------------|--------:|--------:|--------:|--------:|
| allocs/op     | 553,734 | 553,143 | 552,825 | 553,143 |
| p95 wall (ms) | 252     | 241     | 247     | 247     |
| bytes/op      | 192 MB  | 192 MB  | 192 MB  | 192 MB  |

Deltas (median over baseline median):

| Metric          | Review predicts | PoC measured             | Pass? |
|-----------------|-----------------|--------------------------|-------|
| allocs/op delta | −13.6 %         | −81,316 (−12.8 %)        | ✅    |
| p95 wall time   | ≤ baseline      | 264 → 247 ms (−6.4 %)    | ✅    |
| go test ./...   | green           | green, including `-race` | ✅    |

12.8 / 13.6 = 94 % of the predicted saving. The
**pass** gate requires "within 10 %" of the
prediction; we are within 6 %.

Plan 198 is unblocked. It carries the BlockReader fix
forward as a prior win, and tackles the per-parse
arena over `NewTextSegment` + `NewParagraph` +
`Segments.Append` for the remaining ~41 % ceiling.

## Acceptance Criteria

- [x] The review matrix is filled in with every named
      allocator categorised tactical vs structural,
      and the runner-up target is documented.
- [x] The PoC branch builds, tests pass, and the
      benchmark numbers are recorded.
- [x] This plan's Results section has the measured
      delta on the same machine, in the same minute,
      against the main-branch baseline.
- [x] On pass, plan 198 exists and cites the review
      matrix as its work plan.
- [ ] On fail, this plan's Results section names
      what the review missed and the plan is closed
      as ⛔. *Not applicable — PoC passed.*
