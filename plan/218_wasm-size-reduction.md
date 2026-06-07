---
id: 218
title: WASM size reduction — CUE-free engine path and tinygo support
status: "✅"
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

1. [x] Profiled the WASM artifact and confirmed CUE (95 packages) plus
   protobuf (31 packages) were the dominant contributors; ranked the
   package closure by bytes via a native size probe.
2. [x] Built a CUE-free schema/interpolation path behind
   `//go:build wasm`: `internal/fieldinterp` now parses `{field}` paths
   without CUE; `internal/query`, `internal/cuetemplate`, and the
   CUE bits of `internal/schema` / `internal/rules/requiredstructure`
   have native + WASM-stub variants. The artifact dropped from ~38 MB
   to ~10.5 MB raw (~2.7 MB gzipped).
3. [x] Replaced `sync.Map.CompareAndDelete` in
   `internal/lint/runcache.go` with a mutex-guarded compare-and-delete
   so tinygo can compile the package.
4. [x] tinygo build succeeds end to end: ~3 MB. It also needs the
   on-disk `os.Chmod` / `os.SameFile` call sites build-tagged out and
   `-stack-size=1MB` to clear an init-time stack overflow. The smoke
   harness confirms it produces the same diagnostics as native.
5. [x] Tightened `cmd/mdsmith-wasm/size_test.go` to 18 MiB (standard
   Go) and added a tinygo 8 MiB budget test that skips without tinygo.

## Acceptance Criteria

- [x] Standard-Go WASM artifact ≤ 18 MB (~10.5 MB raw, ~2.7 MB gzip).
- [x] `tinygo build -target wasm ./cmd/mdsmith-wasm` succeeds and is
      ≤ 8 MB (~3 MB, with `-stack-size=1MB`).
- [x] Every feature dropped under `//go:build wasm` is documented in
      [the engine-api page](../docs/background/concepts/engine-api.md).
      `Capabilities()` still returns `check`/`fix`/`kinds` — no method
      is dropped — so the per-rule degradations (MDS020 CUE check,
      catalog `where`/CUE rows, `extends` conflict check, index sidecar,
      git-hook writers) are documented there rather than reflected as a
      missing capability.
- [x] `cmd/mdsmith-wasm/size_test.go` asserts the new budgets.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.

## Non-Goals

- Changing the public `pkg/mdsmith` API surface — this is a
  size/compile effort, not an API change.
- The Obsidian plugin UI ([plan 217](217_obsidian-plugin.md)).

## See also

- [Plan 215: engine API and WASM bindings](215_engine-api-wasm.md)
- [Engine API concept page](../docs/background/concepts/engine-api.md)
