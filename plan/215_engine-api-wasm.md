---
id: 215
title: mdsmith public engine API and WASM bindings
status: "✅"
model: opus
summary: >-
  Design `pkg/mdsmith` — a public Go engine API
  with a `Session` that owns workspace, compiled
  config, and parse caches — and mirror it as
  JavaScript bindings in `cmd/mdsmith-wasm/`. Both
  surfaces grow new methods via an open namespace
  plus a `Capabilities()` list. First consumer is
  plan 217 (Obsidian plugin).
depends-on: [163]
---
# mdsmith public engine API and WASM bindings

## Goal

One Go API for the mdsmith engine. Used by the
CLI, the LSP server, and the Obsidian plugin
from [plan 217](217_obsidian-plugin.md).
Mirrored in JavaScript via WebAssembly with the
same method names and shapes.

## Background

mdsmith's engine lives under `internal/` today.
`cmd/mdsmith/` and `internal/lsp/` re-derive
their own plumbing. No public Go surface mirrors
in JS. [Plan 163](163_public-markdown-library.md)
extracted `pkg/markdown`; this plan extends the
same pattern to the engine.

`GOOS=js GOARCH=wasm` has no filesystem, no
subprocess, no threads. The 10 MB Go runtime is
the size cost; `tinygo` trims to 5–8 MB.

## Design

### Public Go API (`pkg/mdsmith`)

```go
package mdsmith

// Session owns workspace, compiled config, and
// per-session caches. Reuse across operations on
// the same workspace.
type Session struct{ /* ... */ }

func NewSession(opts SessionOptions) (*Session, error)

type SessionOptions struct {
    Workspace Workspace    // disk or in-memory
    Config    ConfigSource // path or inline YAML
}

// Core operations
func (s *Session) Check(uri string, source []byte) ([]Diagnostic, error)
func (s *Session) Fix(uri string, source []byte) (FixResult, error)
func (s *Session) Kinds(uri string) (KindResolution, error)

// Introspection and lifecycle
func (s *Session) Capabilities() []string
func (s *Session) Invalidate(uri string, content ...[]byte)
func (s *Session) Dispose()

type Workspace interface {
    ReadFile(path string) ([]byte, error)
    Glob(pattern string) ([]string, error)
}
```

CLI, LSP server, and WASM entry point all
construct via `NewSession`. No parallel internal
API for the same operations.

### WASM bindings (`cmd/mdsmith-wasm`)

`main.go` registers a JS factory that mirrors
`NewSession`:

```go
js.Global().Set("mdsmith", js.ValueOf(map[string]any{
    "createSession": js.FuncOf(createSession),
    "version":       Version,
}))
```

`createSession` returns a session that mirrors each Go method
one-to-one. The methods are `check`, `fix`, `kinds`,
`capabilities`, `invalidate`, and `dispose`. The full TypeScript
shape lives in the
[engine-api concept page](../docs/background/concepts/engine-api.md).

JS method names match the Go names; JS string
arguments cross as Go `[]byte` while URIs stay
strings. `Diagnostic`, `FixResult`, and
`KindResolution` reuse the JSON shapes the LSP
server and `--format json` CLI already emit, so
plan 217 consumes them unchanged. `Diagnostic`
columns are UTF-16 code units (the LSP default),
measured once on the Go side and passed through.

The JS `configYAML` string becomes a Go
`ConfigSource` exactly as the `-c` flag's text
does. `createSession` returns a `Promise` because
`WebAssembly.instantiate` is async, and any
method returning `(T, error)` maps to a
`Promise<T>` that rejects with `new Error(msg)`.
New Go methods reach the JS side in the same
release.

### Open namespace and capabilities

`Session.Capabilities()` returns the method names
this session supports — e.g.
`["check", "fix", "kinds"]` — and JS mirrors it
as `session.capabilities()`. Callers
feature-detect with
`capabilities().includes("extract")` before
calling a method.

The list holds method names, never rule IDs.
Future plans add `extract`, `query`, `deps`,
`rename`, `hover`, `completion` to both sides
without rearranging existing methods. Each new
method declares itself once on Go's `Session`
and once on the proxied JS session — no central
registry.

### Caching

The session owns three caches, all
session-scoped:

- Parsed AST: one entry per URI, holding the
  last `(content-hash, *markdown.Document)`
  pair. Reused when the next `Check` on the
  same URI presents the same content. Cleared
  by `Invalidate(uri)` so old entries don't
  accumulate.
- Compiled config: built once at `NewSession`.
  Config changes require `Dispose()` plus a
  new `NewSession` — there is no in-place
  reconfigure.
- Workspace `ReadFile` results, keyed by path.

`Invalidate(uri)` drops the cached parse and
`ReadFile` bytes for `uri`. With a `content`
argument it also rewrites that file in a
`MemWorkspace` before flushing, so the next
cross-file `Check` reads the new bytes — without
it, an edit to file B leaves file A's view of B
stale. `OSWorkspace` ignores `content` and
re-reads disk; a no-`content` call on a
`MemWorkspace` deletes the entry (file removed).

The first `Check` parses; later `Check` and
`Fix` on the same source skip parse. Plan 217's
steady-state targets depend on this.

### Workspace abstraction

`pkg/mdsmith.Workspace` replaces every
`os.ReadFile` site in `internal/` (roughly 126
today). It is the largest, riskiest piece, so it
lands first. `OSWorkspace` wraps
`os.ReadFile` and `filepath.Glob` for native;
`MemWorkspace`, backed by `map[string][]byte`,
drives WASM and native tests. At WASM session
construction the JS `workspace` map becomes a
`MemWorkspace` once, then mutates only through
`Invalidate(uri, content)`.

`MemWorkspace.Glob` is a linear key filter; the
hot loop must not call it per file. A benchmark
fixture asserts no per-file `Glob` under
`MemWorkspace`.

### MDS040 and build tag

`internal/rules/recipesafety` (MDS040) needs
real shell access. Its `init` registration moves
to a `//go:build !wasm` file, so the package
still compiles under WASM and the blank import
in `internal/rules/all` keeps resolving — the
rule self-registers only on native. `check` runs
the rest; `docs/background/concepts/engine-api.md`
notes MDS040 is out of scope on WASM.

### Build and smoke test

`cmd/mdsmith-wasm/build.sh` runs the trimmed
`go build` against `GOOS=js GOARCH=wasm` and
copies `wasm_exec.js` from
`$(go env GOROOT)/lib/wasm/`.
A second target uses `tinygo`. Both artifacts
land under `cmd/mdsmith-wasm/dist/` for plan
217.

The smoke test runs under
[`wasmbrowsertest`][wbt] or a Node loader. It
creates a session, calls `session.check`, and
asserts the result matches the native CLI.

[wbt]: https://github.com/agnivade/wasmbrowsertest

## Tasks

1. ✅ Doc-only commit sketching `pkg/mdsmith` —
   `Session`, `SessionOptions`, `Workspace`,
   the capability list, method signatures.
   Iterate before implementation.
2. ✅ Add `pkg/mdsmith.Workspace` with
   `OSWorkspace` and `MemWorkspace`. Engine
   read paths and the LSP take a `Workspace`
   (native-only tooling aside); tested against
   `MemWorkspace`.
3. ✅ Build the `Session` type with parse-AST and
   config caches. Each method is a thin shim
   over `internal/engine` and `internal/fix`.
4. 🔳 LSP reads route through `Workspace`;
   fully migrating `cmd/mdsmith/` and
   `internal/lsp/` onto `NewSession` is
   deferred to [plan 219](219_session-cli-lsp-migration.md).
5. ✅ Move the recipesafety `init` registration to
   a `//go:build !wasm` file. Native unaffected;
   WASM omits MDS040.
6. ✅ Add `cmd/mdsmith-wasm/main.go`. Register
   `globalThis.mdsmith.createSession` and
   `globalThis.mdsmith.version`. Built with
   `go`; `tinygo` deferred to
   [plan 218](218_wasm-size-reduction.md).
7. ✅ Smoke test: WASM `session.check` matches
   the native CLI on an in-memory fixture.
8. ✅ Write `docs/background/concepts/engine-api.md`
   — session, caches, open namespace, WASM
   limits, and the ≤ 18 MB / ≤ 8 MB size budgets.
9. ✅ Run `mdsmith fix .` and confirm
   `mdsmith check .` passes.

## Acceptance Criteria

- [x] `pkg/mdsmith` exposes `Session`,
      `NewSession`, `Check`, `Fix`, `Kinds`,
      `Capabilities`, `Invalidate`, `Dispose`,
      plus `Workspace` with `OSWorkspace` and
      `MemWorkspace`. No `os.ReadFile` on the
      engine's read paths survives outside
      `pkg/mdsmith` and `cmd/` (native-only
      tooling aside). Fully routing `cmd/mdsmith`
      and the LSP onto `NewSession` is deferred to
      [plan 219](219_session-cli-lsp-migration.md).
- [x] `cmd/mdsmith-wasm/` builds with
      `GOOS=js GOARCH=wasm`, exporting
      `globalThis.mdsmith.createSession` and
      `globalThis.mdsmith.version`. A test
      asserts the JS session method set matches
      the Go `Session` method set name-for-name.
      tinygo is deferred to
      [plan 218](218_wasm-size-reduction.md); it
      cannot compile the engine today.
- [x] `Capabilities()` returns method names
      (never rule IDs) and returns the same
      list in Go and JS for the same build.
- [x] Repeated `Check(uri, source)` on the
      same source-hash reuses the parsed AST.
      Bench shows steady-state under half the
      cold-start time.
- [x] After `Invalidate(uri, content)` rewrites
      one workspace file, a dependent file's next
      `Check` sees the new bytes; the workspace
      bench shows no per-file `Glob`.
- [x] Smoke test: WASM `check` matches native
      CLI on an in-memory fixture, and runs in
      CI (the `wasm` job, with Node) so it gates.
- [x] The standard-Go WASM artifact ships
      (~38 MB / 8.2 MB gzipped); a size-regression
      test guards it at ≤ 42 MiB raw / 9 MiB
      gzipped. The ≤ 18 MB and `tinygo` ≤ 8 MB
      budgets are deferred to
      [plan 218](218_wasm-size-reduction.md).
- [x] `docs/background/concepts/engine-api.md`
      exists. `mdsmith check .`, `go test ./...`,
      and `go tool golangci-lint run` all pass.

## Non-Goals

- The Obsidian plugin UI — see
  [plan 217](217_obsidian-plugin.md).
- New methods beyond `check`, `fix`, `kinds` —
  the open namespace absorbs them without
  changing this plan.
- WASM builds for `npm`, `pip`, or other
  channels — the artifact targets in-process JS.
- Recipe execution under WASM.
- A standalone WASM playground on the website.

## See also

- [Plan 163: public Markdown library](163_public-markdown-library.md)
- [Plan 65: WASM spike](65_spike-wasm-embedded-inference.md)
- [Plan 217: Obsidian plugin](217_obsidian-plugin.md)
- [Plan 214: hand-rolled LSP bridge (⛔)](214_obsidian-plugin.md)
