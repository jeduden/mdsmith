---
id: 236
title: "cuelite phase 0 — package, façade, and differential harness"
status: "🔲"
model: opus
summary: >-
  Create the public cue/cuelite package — the Value type, the
  CUE delegation pattern, the differential harness (in-house
  path versus the CUE-backed path as oracle), and the benchmark.
  Surface façade methods and call-site migration come in the
  per-surface phases that follow.
depends-on: [215]
---
# cuelite phase 0 — package, façade, and differential harness

## Goal

Stand up the public `cue/cuelite` package as a CUE-backed
scaffold. Add the differential harness and benchmark the later
phases rely on.

## Context

Phase 0 of [plan 218](218_wasm-size-reduction.md); see it for
the full design and strategy. The façade will mirror the CUE
calls mdsmith makes, each delegating to `cuelang.org/go`. Its
methods are added in the per-surface phases. Behaviour matches
CUE, so adopting it later stays green.

## Tasks

1. Create `cue/cuelite` with its `Value` type (wrapping a
   `cue.Value`), the CUE delegation pattern, and path-tagged
   errors. Surface façade methods (`ParsePath`, `Compile`,
   `Unify`, …) are added in their own phases. One unit test per
   function.
2. Build the differential harness: run a value or expression
   through the in-house path and the CUE-backed path, and
   assert identical accept/reject and error field-paths. There
   is no in-house path yet, so it starts as a scaffold.
3. Add the `cue/cuelite`-versus-CUE benchmark.
4. Register `cue/cuelite` in the
   [layering map](../docs/development/architecture/index.md).

## Acceptance Criteria

- [ ] `cue/cuelite` is a public, exported, documented package
      with its `Value` type and delegation scaffold.
- [ ] Each function ships with a dedicated unit test.
- [ ] The differential harness and the benchmark run in CI.
- [ ] No mdsmith call site imports `cue/cuelite` yet.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
- [Plan 215 — engine API and WASM bindings](215_engine-api-wasm.md)
