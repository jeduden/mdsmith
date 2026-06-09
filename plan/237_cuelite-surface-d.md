---
id: 237
title: "cuelite phase 1 — surface D (placeholder paths)"
status: "🔲"
model: sonnet
summary: >-
  Move internal/fieldinterp onto cue/cuelite's ParsePath (still
  delegating to CUE, so green), then flip ParsePath to the
  in-house parser, checked against the CUE-backed path. Surface
  D is the smallest surface and proves the adopt-then-flip
  cadence end to end.
depends-on: [236]
---
# cuelite phase 1 — surface D (placeholder paths)

## Goal

Move placeholder-path parsing onto `cue/cuelite`, then flip
`ParsePath` to the in-house parser.

## Context

Phase 1 of [plan 218](218_wasm-size-reduction.md). Surface D
uses only `cue.ParsePath`, to parse paths like `{a.b.c}` and
`{"my-key".sub}`; resolution is already hand-rolled. It is the
smallest surface, so it proves the cadence before the larger
flips.

## Tasks

1. Add `ParsePath` to the `cue/cuelite` façade, delegating to
   `cue.ParsePath`.
2. Move [fieldinterp](../internal/fieldinterp/fieldinterp.go)
   onto `cuelite.ParsePath`; drop its `cuelang.org/go` import.
   The suite stays green, because behaviour is unchanged.
3. Flip `cuelite.ParsePath` to an in-house path parser, with
   red/green unit tests and the differential harness checking
   it against the CUE-backed path.

## Acceptance Criteria

- [ ] `internal/fieldinterp` imports `cue/cuelite`, not
      `cuelang.org/go`.
- [ ] `cuelite.ParsePath` is in-house and the harness shows it
      matches CUE on the path corpus.
- [ ] `cue/cuelite` path code keeps 100 % statement and branch
      coverage.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## Implementation notes

- **Extend the existing harness, don't fork it.** Surface D adds
  its path-parse case by extending `internal/cuelitetest`'s `Case`
  and `Outcome` (a new case field plus a stage or payload as the
  shape needs), not by standing up a parallel structure. `Outcome.
  Equal` now compares `Paths` at every stage, so a parsed-segment
  payload attached at `StageAccepted` is differentially checked
  rather than silently always-equal.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
