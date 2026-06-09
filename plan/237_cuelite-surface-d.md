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

- **`Path` is not opaque — both directions are real consumers.**
  [`fieldinterp.ParseCUEPath`](../internal/fieldinterp/fieldinterp.go)
  reads the unquoted per-selector strings back OUT of a parsed path
  (the harness compares `[][]string`), so the surface-D `Path` API
  must expose a `Segments()` accessor.
  [`query.collectPaths`](../internal/query/query.go) builds paths IN
  programmatically via `cue.MakePath` + `iter.Selector()`, so the
  API must also offer a constructor-from-segments (the in-house
  `MakePath` equivalent). Adopt both when adding `ParsePath`, so the
  flip in task 3 does not change the API.
- **Extend the harness TYPES, but add a separate path arm — don't
  append to the corpus.** Surface D extends `internal/cuelitetest`'s
  `Case` and `Outcome` (a new case field plus a stage or payload as
  the shape needs), not a parallel structure. But `Run` hardcodes
  the schema/data arms (`CueLitePath`/`OraclePath`), and a path-only
  `Case` in the existing `corpus()` slice would agree VACUOUSLY —
  an empty schema and data classify identically in both arms
  regardless of the path. So surface D adds its OWN path-comparing
  arm/runner (parse via cuelite vs `cue.ParsePath`, compare
  segments) rather than appending path cases to the existing corpus
  slice. `Outcome.Equal` already compares `Paths` at every stage, so
  a parsed-segment payload is differentially checked there.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
