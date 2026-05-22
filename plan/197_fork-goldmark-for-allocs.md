---
id: 197
title: PoC — pool one goldmark allocator and measure the savings
status: "🔲"
model: opus
depends-on: [195]
summary: >-
  Before plan 198 commits to forking the goldmark parser, prove
  the approach delivers the alloc savings the profile predicts.
  Vendor the minimum subset of goldmark needed to pool one
  allocator (text.Segments — 13.8 % of every check's allocs),
  apply the pool, and measure. If allocs/op drops by the
  expected 1.26 M objects on BenchmarkCheckCorpusLarge AND wall
  time also drops (pools that trade allocs for sync.Pool
  overhead are theatre), schedule the full fork as plan 198. If
  the savings are smaller than predicted, or wall time
  regresses, stop and document what the profile missed.
---
# PoC — pool one goldmark allocator and measure the savings

## Goal

Answer one question with a real measurement. Does
pooling a hot goldmark allocator deliver the alloc and
wall-time savings the plan-195 profile predicts?

The deliverable is a number, not a shipped fork. The
PoC branch is throwaway. The plan ships a decision —
write plan 198 (the full fork) or abandon the approach.

## Background

[Plan 195](195_per-rule-alloc-budget.md)'s profile of
`BenchmarkCheckCorpusLarge` attributes 55 % of every
check's allocations to five goldmark parser
allocators. The top one alone —
`goldmark/text.(*Segments).Append` — is 1.26 M
allocation objects per 10-iteration bench run, 13.8 %
of the total. A full fork would patch all five.

A full fork is a multi-week investment with a
significant maintenance tail. Before paying that
cost, prove the leverage works.

The pattern is well-known but the result is not
guaranteed. Pools sometimes trade one allocation for
sync.Pool's own overhead. The AST may carry hidden
invariants the patch breaks. The equivalence harness
may not be buildable without changes the PoC has not
scoped.

The PoC picks **`text.Segments`** as the target. The
choice is deliberate:

- It is the second-largest allocator in the profile.
- The data is a `[]Segment` slice — the pool primitive
  is trivial (`sync.Pool` of `*Segments`).
- The lifecycle is bounded by a parse: every Segments
  list is owned by exactly one paragraph and consumed
  by exactly one rule. Pool aliasing is contained.

[`pkg/markdown`](../pkg/markdown/) is the public
surface the PoC patches behind. Plan 175 extracted it
already; the PoC's parser swap lives behind that API
so no caller changes.

## Approach

Single throwaway branch. Three commits inside it.

### Commit 1: vendor the minimum subset

Copy only the goldmark files that reach `text.Segments`
into `internal/goldmark/`. The set is bounded by
`go build` — anything that resolves under that subset
is in; anything else is out. A whole-tree vendor is
out of scope for the PoC.

Update mdsmith's imports of the touched packages to
point at `internal/goldmark/...`. Untouched goldmark
packages (e.g. extensions mdsmith does not reach) stay
on the upstream import.

The PoC does not bother with a build tag or an
upstream A/B path. The throwaway branch is the
upstream-fork delta on its own.

### Commit 2: apply the pool

Add a `sync.Pool` of `*Segments` to
`internal/goldmark/text/`. `NewSegments` gets from
the pool, `Reset()` puts back, `Append()` is
unchanged.

Wire the parser to call `Reset` on every Segments at
paragraph close so the backing returns to the pool.

The pool is per-goroutine via sync.Pool's standard
shape — no explicit scoping needed.

### Commit 3: measure

Run:

- `BenchmarkCheckCorpusLarge -benchtime=10x` against
  the PoC branch. Record allocs/op and p95 wall time.
- `BenchmarkCheckCorpusLarge -benchtime=10x` against
  the main branch (the pre-PoC baseline). Same
  machine, same run, same minute.
- `go test ./...` on the PoC branch — every existing
  test must still pass. If any test fails, the AST
  drift is the answer and the PoC stops there.

Compare:

- **alloc delta**: expected ≥ 1 M objects/op on the
  10x bench. The profile attributes 1.26 M; allow
  some slack for the pool's bookkeeping.
- **time delta**: expected ≤ 0 ms (no regression).
  The pool's whole job is to avoid the heap; if it
  trades allocs for time, the patch is theatre and
  the full fork is not worth doing.

## Tasks

1. [ ] Survey which goldmark files touch
   `text.Segments`. Record the file list as the
   vendor manifest for the PoC.
2. [ ] Create a throwaway branch
   `claude/poc-pool-goldmark-segments`. Land
   commit 1 (vendor the manifest, update imports,
   confirm `go build ./...` is clean and
   `go test ./...` passes).
3. [ ] Land commit 2 (the pool + Reset wiring). Run
   `go test ./...` again. Every test still passes or
   the PoC stops; record the failure and exit.
4. [ ] Land commit 3 (the measurement). Capture the
   alloc/op and p95 wall-time numbers for both the
   PoC branch and main, side by side.
5. [ ] Update this plan with the PoC results. Three
   columns: profile prediction, measured PoC delta,
   pass/fail. Pass means alloc savings ≥ 1 M and
   wall time ≤ baseline. Fail means the data
   contradicts the approach.
6. [ ] On pass, write plan 198 — the full fork —
   citing the PoC numbers as the justification.
7. [ ] On fail, write the rationale into this plan's
   Results section and close it as ⛔ (superseded by
   the rationale's preferred alternative).

## Risk

The PoC scope is one allocator. If the savings for
that allocator are real, the other four (NewTextSegment,
NewBlockReader, NewParagraph, newLinkLabelState) likely
follow the same shape. If the savings are not real, the
others would not have helped either and the full fork
is the wrong move.

Pool aliasing is the second risk. A Segments returned
to the pool while the rule still references it would
silently corrupt the AST. The fix is the existing
pattern from plan 193 (`internal/punkt`): pool only
within the parser's own scope and document the
"do not retain past Parse" contract. The mdsmith rule
packages all consume nodes inside one `Parser` call,
so the contract holds in production today.

The PoC vendor manifest is a subset of the full fork.
The full fork would also patch the other allocators
and likely vendor more files. The PoC's vendor list
is not the full-fork's vendor list and must not be
mistaken for it.

## Results

To be filled in by task 5.

| Metric          | Profile predicts       | PoC measured | Pass? |
|-----------------|------------------------|--------------|-------|
| allocs/op delta | −1.26 M (≥ −1.0 M)     |              |       |
| p95 wall time   | no regression (≤ 0 ms) |              |       |
| go test ./...   | green                  |              |       |

## Acceptance Criteria

- [ ] The PoC branch builds, tests pass, and the
      benchmark numbers are recorded.
- [ ] This plan's Results section is filled in with
      the measured delta on the same machine, taken
      in the same minute, against the main-branch
      baseline.
- [ ] On a pass, plan 198 exists and cites the PoC
      numbers in its justification.
- [ ] On a fail, this plan's Risk + Results sections
      document what the profile missed and the plan
      is closed as ⛔.
