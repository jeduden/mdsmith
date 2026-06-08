---
id: 240
title: "cuelite phase 4 — drop cuelang.org and enable tinygo"
status: "🔲"
model: sonnet
summary: >-
  With every surface flipped, delete cue/cuelite's CUE
  delegation and remove cuelang.org/go from go.mod; replace the
  tinygo-incompatible sync.Map.CompareAndDelete in
  internal/lint/runcache.go; get the standard-Go and tinygo
  WASM builds under the plan-215 budgets; update the engine-api
  page and the layering map.
depends-on: [239]
---
# cuelite phase 4 — drop cuelang.org and enable tinygo

## Goal

Remove `cuelang.org/go` entirely. Make the WASM artifact fit
the plan-215 budgets on the standard-Go and tinygo toolchains.

## Context

Phase 4 of [plan 218](218_wasm-size-reduction.md). It is
reachable once surfaces A–D are flipped, so nothing delegates
to CUE anymore.

## Tasks

1. Delete the CUE delegation from `cue/cuelite` and remove
   `cuelang.org/go` from `go.mod` and `go.sum`. Confirm no
   non-test file imports `cuelang.org/...`.
2. Replace `sync.Map.CompareAndDelete` in
   [runcache.go](../internal/lint/runcache.go) with a
   mutex-guarded map, red/green.
3. Get the standard-Go and `tinygo build -target wasm
   ./cmd/mdsmith-wasm` builds passing; tighten
   [size_test.go](../cmd/mdsmith-wasm/size_test.go) to the new
   budgets.
4. Update the
   [engine-api page](../docs/background/concepts/engine-api.md)
   and the `cue/` entry in the
   [layering map](../docs/development/architecture/index.md).

## Acceptance Criteria

- [ ] `cuelang.org/go` is absent from `go.mod` and `go.sum`;
      no non-test file imports `cuelang.org/...`.
- [ ] Standard-Go WASM artifact ≤ 18 MB.
- [ ] `tinygo build -target wasm ./cmd/mdsmith-wasm` succeeds
      and is ≤ 8 MB; `size_test.go` asserts the new budgets.
- [ ] `Capabilities()` is unchanged.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
