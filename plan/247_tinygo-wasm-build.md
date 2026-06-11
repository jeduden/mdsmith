---
id: 2606110517
title: "tinygo wasm build under the 8 MiB budget"
status: "✅"
model: sonnet
summary: >-
  Make `tinygo build -target wasm ./cmd/mdsmith-wasm` succeed and
  fit the plan-215/218 ≤ 8 MiB budget by isolating the four
  standard-library calls tinygo's wasm target lacks — os.Chmod,
  os.SameFile, os.Symlink, filepath.EvalSymlinks — behind
  build-tagged seams per package, then un-skip the size budget
  test and flip the CI tinygo-wasm job to enforcing.
depends-on: [240]
---
# tinygo wasm build under the 8 MiB budget

## Goal

Make `tinygo build -target wasm ./cmd/mdsmith-wasm` compile. The
artifact must fit the plan-215/218 budget of 8 MiB.

## Context

This is the one remaining criterion from
[plan 240](240_cuelite-drop-cue.md). Dropping `cuelang.org/go`
cleared two earlier walls. The `sync.Map.CompareAndDelete` swap
cleared a third. The tinygo build still fails.

The failure is not size. It is four standard-library calls
tinygo's wasm target does not implement. Each one is reached
transitively from
[pkg/mdsmith](../docs/background/concepts/engine-api.md).

The real build attempt under tinygo 0.39.0 named this inventory:

- `os.Chmod` — [internal/schema/index.go:231](../internal/schema/index.go),
  [internal/fix/fix.go:689](../internal/fix/fix.go), and
  [internal/githooks](../internal/githooks/).
- `os.SameFile` — [internal/githooks](../internal/githooks/).
- `os.Symlink` and `filepath.EvalSymlinks` —
  [internal/schema](../internal/schema/),
  [internal/lsp](../internal/lsp/), and the cross-file rules.

The standard `GOOS=js` wasm path must stay unchanged. Only the
tinygo build needs the seams.

## Tasks

1. Add a build-tagged seam per package for each missing call. A
   non-tinygo file keeps the current body. A tinygo file supplies
   the variant. Pick the variant semantics per call site.

2. For `os.Chmod` on a freshly written index or fixed file, the
   tinygo variant is a no-op. The wasm host has no POSIX mode
   bits. Skipping the chmod degrades nothing the engine reads
   back.

3. For `os.SameFile` and the symlink calls in
   [internal/githooks](../internal/githooks/), the tinygo variant
   degrades. The hook-install path is not reachable from the wasm
   surface. The variant returns a "not supported" error.

4. For `os.Symlink` and `filepath.EvalSymlinks` in
   [internal/schema](../internal/schema/),
   [internal/lsp](../internal/lsp/), and the cross-file rules, the
   tinygo variant skips symlink resolution. The wasm sandbox has
   no symlinks. An identity resolution is correct there.

5. Un-skip `TestTinyGoWASMArtifactSizeBudget` in
   [cmd/mdsmith-wasm/size_test.go](../cmd/mdsmith-wasm/size_test.go).
   Replace the known-failure skip with the size assertion against
   `maxTinyGoWASMBytes`.

6. Flip the `tinygo-wasm` job in
   [.github/workflows/ci.yml](../.github/workflows/ci.yml) to
   enforcing. Remove `continue-on-error: true`. Remove the comment
   that points here.

7. Update the
   [engine-api page](../docs/background/concepts/engine-api.md)
   and [plan 218](218_wasm-size-reduction.md). Record the tinygo
   build as CI-verified, not scheduled.

## Acceptance Criteria

- [x] `tinygo build -target wasm ./cmd/mdsmith-wasm` succeeds with
      tinygo 0.39.0.
- [x] The tinygo artifact is at or under 8 MiB.
      `TestTinyGoWASMArtifactSizeBudget` asserts it rather than
      skipping.
- [x] The standard `GOOS=js` wasm build is unchanged.
      `TestWASMArtifactSizeBudget` still passes.
- [x] The CI `tinygo-wasm` job enforces the build. A regression
      fails the PR.
- [x] No call site loses its standard-library behaviour on a
      non-tinygo build.
- [x] Plan 240, plan 218, and the engine-api page read truthfully.
      The tinygo build is verified, not pending.
- [x] `go test ./...` and `go run ./cmd/mdsmith check .` pass.

## See also

- [Plan 240 — drop cuelang.org and enable tinygo](240_cuelite-drop-cue.md)
- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
- [Plan 215 — engine API and WASM bindings](215_engine-api-wasm.md)
