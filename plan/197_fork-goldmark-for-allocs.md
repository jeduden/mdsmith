---
id: 197
title: PoC — review goldmark's allocation architecture, then pool the best lever
status: "🔲"
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

Read goldmark's parser end-to-end. For every
allocation site the plan-195 profile names, fill out:

- **Lifecycle**: per-document, per-block, per-line,
  per-segment, per-token.
- **Reuse barrier**: what stops a single instance from
  being shared across calls (e.g. unexported state,
  pointer escapes to AST node, mutated by caller).
- **Category**:
  - **Tactical**: type is already reuse-friendly
    (has `Reset()`, no escape); a pool slot eats the
    cost.
  - **Structural**: type design forces per-call
    allocation; a refactor (arena, struct-of-arrays,
    parser-shared instance) is needed for the win.
- **Estimated saving**: the alloc-count drop the fix
  would deliver per 10x bench, derived from the
  profile attribution.
- **Risk**: AST aliasing? Pool contention? API
  break?

Record the matrix in this plan as the "review
matrix" table.

Cross-cutting questions the review answers
explicitly:

- Could a single per-parse arena replace four of the
  five hot allocators?
- Does the link-reference transformer's per-paragraph
  BlockReader allocation persist any state, or could
  one parser-shared BlockReader cover every paragraph
  via Reset?
- Are there structural opportunities the plan-195
  profile missed because they show as "small flat
  allocs across many call sites"?

### Stage two — PoC the biggest lever

Rank the review matrix by estimated saving. Pick the
single highest-saving target. Implement just that one
on a throwaway branch.

If the target is tactical (a pool):

- Vendor the minimum goldmark files into
  `internal/goldmark/`.
- Add the pool.
- Wire Reset on the release path.

If the target is structural (an arena or shared
instance):

- Vendor the minimum subset.
- Refactor the allocation site to use the new shape.
- Add the cleanup hook (arena reset on parse end,
  shared instance reset between calls).

Either way, the PoC does not bother with a build
tag, an equivalence harness, or an upstream A/B path.
Those are full-fork costs, deferred to plan 198.

### Stage three — measure and decide

Run side by side against the pre-PoC main branch:

- `BenchmarkCheckCorpusLarge -benchtime=10x` — allocs
  and p95 wall time.
- `go test ./...` — every existing test passes or the
  PoC stops; the test failure is the answer.

Compare against the review matrix's prediction:

- **Pass** = alloc savings within 10 % of the matrix
  prediction AND wall time ≤ baseline. Pools that
  trade allocs for sync.Pool overhead are theatre;
  the gate refuses the trade.
- **Fail** = either condition false. The Results
  section explains which (and what the review
  missed).

Write plan 198 on a pass, with the review matrix as
the work plan and the PoC numbers as the
justification. Close 197 as ⛔ on a fail.

## Tasks

1. [ ] Read `goldmark/parser/parser.go`,
   `goldmark/parser/link_ref.go`, `goldmark/text/reader.go`,
   `goldmark/text/segments.go`, `goldmark/ast/*.go`,
   and any extension under `goldmark/extension/` that
   the engine bench reaches. Note the lifecycle and
   reuse-barrier for every allocation site the
   plan-195 profile names.
2. [ ] Build the review matrix below. One row per
   allocator. Columns: lifecycle, reuse barrier,
   category, estimated saving, risk.
3. [ ] Answer the cross-cutting questions in a short
   "review findings" subsection.
4. [ ] Rank by estimated saving. Pick the highest.
   Document the choice and the runner-up so the
   alternative is on record.
5. [ ] Create a throwaway branch
   `claude/poc-goldmark-<chosen-target>`. Vendor the
   minimum goldmark subset the change touches.
   `go build ./...` and `go test ./...` must stay
   green.
6. [ ] Implement the chosen change. Run `go test ./...`
   again. Any failure stops the PoC and gets recorded
   in Results.
7. [ ] Capture the side-by-side bench numbers (allocs
   and p95) against the pre-PoC baseline. Same
   machine, same minute.
8. [ ] Fill in this plan's Results section with the
   review prediction and the PoC measured numbers.
9. [ ] On pass, write plan 198 — the full fork —
   carrying the review matrix forward as its work
   plan and the PoC numbers as the justification.
10. [ ] On fail, close 197 as ⛔ and write the
    rationale into the Results section.

## Review matrix

To be filled in by task 2. Skeleton:

| Allocator         | Lifecycle | Reuse barrier | Category | Est. saving | Risk |
|-------------------|-----------|---------------|----------|-------------|------|
| NewTextSegment    |           |               |          |             |      |
| Segments.Append   |           |               |          |             |      |
| NewBlockReader    |           |               |          |             |      |
| NewParagraph      |           |               |          |             |      |
| newLinkLabelState |           |               |          |             |      |

## Risk

The review can miss things. The matrix only covers
allocators the plan-195 profile already named; a
review that looks only at those misses any structural
shape that shows as "tens of small allocs across
many sites". Mitigation: the cross-cutting questions
include "did the profile miss anything?" so the
reviewer explicitly looks beyond the top-5.

The PoC scope is one change. If that change is
tactical and delivers, the structural changes may
deliver more. If the chosen change is structural and
delivers, the tactical pools may not pay off — pool
overhead can erase the alloc savings on an already
fast allocator. The Results section names both for
plan 198 to weigh.

Pool aliasing is the standard risk the plan-193
precedent already names. The mdsmith rule packages
consume AST nodes inside one `Parser` call, so the
"do not retain past Parse" contract holds in
production today. The PoC's chosen change inherits
that contract.

## Results

To be filled in by task 8.

| Metric          | Review predicts | PoC measured | Pass? |
|-----------------|-----------------|--------------|-------|
| allocs/op delta |                 |              |       |
| p95 wall time   |                 |              |       |
| go test ./...   |                 |              |       |

## Acceptance Criteria

- [ ] The review matrix is filled in with every named
      allocator categorised tactical vs structural,
      and the runner-up target is documented.
- [ ] The PoC branch builds, tests pass, and the
      benchmark numbers are recorded.
- [ ] This plan's Results section has the measured
      delta on the same machine, in the same minute,
      against the main-branch baseline.
- [ ] On pass, plan 198 exists and cites the review
      matrix as its work plan.
- [ ] On fail, this plan's Results section names
      what the review missed and the plan is closed
      as ⛔.
