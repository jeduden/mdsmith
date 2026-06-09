---
id: 236
title: "cuelite phase 0 — package, façade, and differential harness"
status: "✅"
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

- [x] `cue/cuelite` is a public, exported, documented package
      with its `Value` type and delegation scaffold.
- [x] Each function ships with a dedicated unit test.
- [x] The differential harness and the benchmark run in CI.
- [x] No mdsmith call site imports `cue/cuelite` yet.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.

## Implementation notes

Two choices differ from the sketch in plan 218, both
behaviour-preserving:

- **Shared context, no per-`Value` rebinding.** Every `Value`
  compiles against one package-wide `*cue.Context`, so `Unify`
  needs no cross-context re-binding. This replaces the per-`Value`
  context pairing the sketch implied, and matches the
  `internal/schema` rule that two values must share a context to
  unify. The flip to the in-house engine drops the shared context
  entirely, as planned.
- **Harness lives in `cue/cuelite/difftest`.** The differential
  harness is a sibling sub-package, not an in-package file. It
  exposes `CueLitePath` (the in-house path), `OraclePath` (the CUE
  oracle), and `Run` over a `Case` corpus, with a CI-visible
  `TestRun_corpus`. The later phases plug the real engine into
  `CueLitePath` unchanged.

The phase-0 surface is small. It has `Compile`, `CompileJSON`,
`Value.Unify`, and `Value.Validate`. It also has the `PathError`
type with `NewPathError`, `Path`, and `Error`. The rest of the
façade arrives in the per-surface phases.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
- [Plan 215 — engine API and WASM bindings](215_engine-api-wasm.md)
