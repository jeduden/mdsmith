---
id: 215
title: mdsmith WASM build target and Workspace abstraction
status: "🔲"
model: opus
summary: >-
  Compile mdsmith to WebAssembly with `GOOS=js
  GOARCH=wasm`, refactor `internal/` file reads
  behind a `Workspace.ReadFile` interface so the
  WASM build runs without a host filesystem, and
  expose a small JS API
  (`globalThis.mdsmith.{check,fix,kinds,version}`)
  the Obsidian plugin from plan 217 consumes.
depends-on: []
---
# mdsmith WASM build target and Workspace abstraction

## Goal

Make mdsmith runnable inside a JavaScript host
that has no filesystem and no subprocess support
— the Obsidian plugin from
[plan 217](217_obsidian-plugin.md) is the first
consumer. A future consumer could be a website
playground or a browser-based editor.

This plan ships the Go side. It exposes
mdsmith's check, fix, and kinds entry points as
plain JS functions, and refactors every
`os.ReadFile` site in `internal/` so the WASM
build can read from an injected workspace map.
The plugin UI (scaffolding, diagnostics, code
actions, settings tab) is
[plan 217](217_obsidian-plugin.md).

## Background

mdsmith currently reads files through
`os.ReadFile`. WASM under `GOOS=js GOARCH=wasm`
has no `os` filesystem support and no
`os/exec`. The
[plan 65 spike](65_spike-wasm-embedded-inference.md)
demonstrated the build path works. The cost is
binary size (Go's runtime adds about 10 MB even
with trimming) and the loss of `os.Exec`, raw
threads, and stdio.

`tinygo` lands a smaller bundle (5–8 MB) at the
cost of standard-library coverage and slower
build times. Task 4 builds both and the plan
completion note records which toolchain ships.

## Design

### Workspace abstraction

A new package `internal/workspace`:

```go
type Workspace interface {
    ReadFile(path string) ([]byte, error)
    Glob(pattern string) ([]string, error)
}
```

Two implementations:

- `OSWorkspace` — wraps `os.ReadFile` and
  `filepath.Glob`. The native build uses it.
- `MemWorkspace` — backed by
  `map[string][]byte`. The WASM build uses
  it; the native build uses it in tests.

Every `internal/` site that calls `os.ReadFile`
or walks the filesystem takes a `Workspace`
parameter or pulls one from a context-scoped
provider. The refactor is the riskiest piece.
Task 1 lands it before the WASM build does.

The hot loop must not call `Glob` per file —
the snapshot already enumerated every input.
`MemWorkspace.Glob` is a linear key filter.

### WASM build target

`cmd/mdsmith-wasm/main.go`:

```go
js.Global().Set("mdsmith", js.ValueOf(map[string]any{
    "check":   js.FuncOf(check),
    "fix":     js.FuncOf(fix),
    "kinds":   js.FuncOf(kinds),
    "version": Version,
}))
```

- `check(uri, source, workspaceMap, configYAML)`
  → JS array of diagnostics shaped like the
  LSP `Diagnostic` payload.
- `fix(uri, source, workspaceMap, configYAML)`
  → `{ output: string, edits: TextEdit[] }`.
- `kinds(configYAML)` mirrors the CLI.
- `version` is the `Version` constant the
  release pipeline stamps.

Each entry point builds a `MemWorkspace` from
the JS map, then calls the existing
`internal/engine` and `internal/fix` paths
unchanged.

### MDS040

MDS040 (recipe shell scanning) needs real
shell access. The WASM runtime skips it. A
constant in `internal/buildflags` (or
equivalent) tags the WASM build so the rule
registry can filter MDS040 out. Native builds
keep MDS040 enabled.

### Build

`cmd/mdsmith-wasm/build.sh` (or a Make target)
runs:

```bash
GOOS=js GOARCH=wasm go build \
  -trimpath -ldflags="-s -w" \
  -o dist/mdsmith.wasm \
  ./cmd/mdsmith-wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" \
  dist/wasm_exec.js
```

A second target builds with `tinygo`. Both
artifacts land under `cmd/mdsmith-wasm/dist/`
for plan 217 to pick up.

### Smoke test

Tests run under [`wasmbrowsertest`][wbt] or a
Node loader. The smoke test loads the WASM,
calls `mdsmith.check` against an in-memory
`{ "doc.md": "# Title\nbody" }` input, and
asserts the diagnostics array matches the
native `mdsmith check` output for the same
input.

[wbt]: https://github.com/agnivade/wasmbrowsertest

### Bundle size

Two budgets gate the plan:

- Standard Go WASM trimmed: target ≤ 18 MB.
- `tinygo` WASM trimmed: target ≤ 8 MB.

Task 4 measures both and records the chosen
one in the plan completion note.

### Docs

A new background page explains the runtime
choice. It also lists what WASM cannot do
(no shell, no host filesystem, MDS040
skipped) and names the size cap. The same
page documents the `Workspace` interface so
contributors know which sites take one and
which still use the OS directly.

## Tasks

1. Introduce `internal/workspace.Workspace`
   with `OSWorkspace` and `MemWorkspace`
   implementations. Refactor every
   `os.ReadFile` site in `internal/` to take
   a `Workspace`. Each site keeps its tests
   green. One new test injects a
   memory-backed workspace.
2. Add the `internal/buildflags` constant
   for the WASM build. Make the rule
   registry filter MDS040 when the flag is
   set.
3. Add `cmd/mdsmith-wasm/main.go`. Register
   `globalThis.mdsmith.{check,fix,kinds,version}`.
   Build a `MemWorkspace` from the JS
   workspace map per call.
4. Write `cmd/mdsmith-wasm/build.sh` (or
   equivalent Make target) that produces
   both the standard Go and `tinygo`
   artifacts. Record the smaller correct one
   in the plan completion note.
5. Run a smoke test under `wasmbrowsertest`
   or a Node loader: instantiate the WASM,
   call `mdsmith.check` on an in-memory
   fixture, and assert the diagnostics array
   matches the native `mdsmith check` output
   for the same input.
6. Write
   `docs/background/concepts/wasm-build.md`.
   Document the runtime choice, the WASM
   constraints, and the `Workspace`
   abstraction.
7. Run `mdsmith fix .` and confirm
   `mdsmith check .` passes.

## Acceptance Criteria

- [ ] `cmd/mdsmith-wasm/` builds with
      `GOOS=js GOARCH=wasm`. The trimmed
      artifact exports
      `globalThis.mdsmith.{check,fix,kinds,version}`.
- [ ] A `tinygo` build of
      `cmd/mdsmith-wasm/` also succeeds. The
      plan completion note records which
      toolchain ships.
- [ ] Every `internal/` site that read
      through `os.ReadFile` now takes a
      `Workspace`. Every existing test still
      passes against `OSWorkspace`. One new
      test runs the same path against
      `MemWorkspace`.
- [ ] Smoke test under a Node or browser
      WASM loader passes. The WASM build's
      `check` returns the same diagnostics
      as the native CLI for an in-memory
      fixture.
- [ ] MDS040 returns no findings under the
      WASM build but still runs under the
      native build.
- [ ] Standard Go WASM trimmed stays ≤
      18 MB. `tinygo` WASM trimmed stays ≤
      8 MB.
- [ ] `docs/background/concepts/wasm-build.md`
      exists. It documents the runtime, the
      constraints, and the `Workspace`
      abstraction.
- [ ] `mdsmith check .` passes; `go test
      ./...` passes; `go tool golangci-lint
      run` is clean.

## Non-Goals

- The Obsidian plugin UI. That is
  [plan 217](217_obsidian-plugin.md).
- A WASM build for `npm`, `pip`, or other
  channels. The artifact targets in-process
  JS hosts.
- Recipe execution under WASM. MDS040 is the
  only affected rule; it skips silently.
- A standalone WASM playground on the
  website. The artifact is a building block,
  not a hosted demo.
- Refactoring rule code beyond the
  `Workspace.ReadFile` swap. Rules that walk
  the AST stay the same.

## See also

- [Plan 65: WASM spike](65_spike-wasm-embedded-inference.md)
- [Plan 217: Obsidian plugin](217_obsidian-plugin.md)
- [Plan 214: hand-rolled LSP bridge (⛔)](214_obsidian-plugin.md)
