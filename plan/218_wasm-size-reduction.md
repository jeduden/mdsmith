---
id: 218
title: WASM size reduction — CUE-free engine path and tinygo support
status: "🔲"
model: opus
summary: >-
  Bring the `cmd/mdsmith-wasm` artifact under the ≤ 18 MB (standard
  Go) and ≤ 8 MB (tinygo) budgets plan 215 set but could not reach.
  The engine pulls in CUE (95 packages) plus protobuf via
  `internal/schema`, `internal/fieldinterp`, and `internal/query`; a
  CUE-free schema/interpolation path and a tinygo-compatible
  `internal/lint` runcache are the two levers.
depends-on: [215]
---
# WASM size reduction — CUE-free engine path and tinygo support

## Goal

Shrink the mdsmith WebAssembly artifact to the plan-215 size
targets: ≤ 18 MB standard-Go and ≤ 8 MB tinygo. It is ~38 MB today
(8.2 MB gzipped). A smaller bundle lets the
[Obsidian plugin (plan 217)](217_obsidian-plugin.md) run on mobile.

## Background

[Plan 215](215_engine-api-wasm.md) delivered the public engine API
and a working standard-Go WASM build. But that artifact runs ~2×
over budget. And tinygo cannot compile the engine at all. Two root
causes, both diagnosed during plan 215:

- **CUE + protobuf bulk.** `internal/schema` (MDS020 file-schema
  validation), `internal/fieldinterp` (catalog/include field
  interpolation), and `internal/query` pull in CUE's ~95 packages
  plus protobuf. This is the dominant size cost and cannot be
  build-tagged out without disabling those features.
- **tinygo incompatibility.** tinygo's standard library omits
  `sync.Map.CompareAndDelete`, used by
  [`internal/lint/runcache.go`](../internal/lint/runcache.go), and
  CUE's heavy reflection blocks compilation further.

Plan 215 guards the artifact at its real size (≤ 42 MiB raw /
≤ 9 MiB gzipped) so it cannot grow silently; this plan attacks the
budget itself.

## Design

Two independent levers, either of which helps:

1. **CUE-free fast path.** Behind a `//go:build wasm` tag, replace
   the CUE-backed schema compile and field interpolation with a
   hand-rolled evaluator covering the subset a WASM host needs.
   Measure which features (MDS020, catalog `where`, query) the
   Obsidian host actually calls and gate the rest out.
2. **tinygo-compatible runcache.** Replace the
   `sync.Map.CompareAndDelete` use in
   [`internal/lint/runcache.go`](../internal/lint/runcache.go) with a
   mutex-guarded map the tinygo runtime supports, then re-attempt the
   tinygo build once CUE is gone.

## Tasks

1. Profile the WASM artifact (`go tool nm -size`) to confirm CUE and
   protobuf are the dominant contributors and rank packages by bytes.
2. Prototype a CUE-free schema/interpolation path behind
   `//go:build wasm`; measure the size drop.
3. Remove `sync.Map.CompareAndDelete` from the lint runcache so
   tinygo can compile that package.
4. Attempt the tinygo build end to end; record the artifact size.
5. Tighten the plan-215 size-budget test to the budgets this plan
   reaches.

## Acceptance Criteria

- [ ] Standard-Go WASM artifact ≤ 18 MB.
- [ ] `tinygo build -target wasm ./cmd/mdsmith-wasm` succeeds and is
      ≤ 8 MB.
- [ ] Any feature dropped under `//go:build wasm` is documented in
      [the engine-api page](../docs/background/concepts/engine-api.md)
      and reflected by `Capabilities()`.
- [ ] `cmd/mdsmith-wasm/size_test.go` asserts the new budgets.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## Non-Goals

- Changing the public `pkg/mdsmith` API surface — this is a
  size/compile effort, not an API change.
- The Obsidian plugin UI ([plan 217](217_obsidian-plugin.md)).

## See also

- [Plan 215: engine API and WASM bindings](215_engine-api-wasm.md)
- [Engine API concept page](../docs/background/concepts/engine-api.md)
