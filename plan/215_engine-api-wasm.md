---
id: 215
title: mdsmith public engine API and WASM bindings
status: "🔲"
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
the size cost. `tinygo` trims to 5–8 MB; the
build target builds both and the completion
note records which ships.

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
func (s *Session) Invalidate(uri string)
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

The session returned by `createSession` carries
each Go method one-to-one:

```ts
declare const mdsmith: {
  createSession(opts: SessionOptions): Session;
  version: string;
};

interface Session {
  check(uri: string, src: string): Promise<Diagnostic[]>;
  fix(uri: string, src: string): Promise<FixResult>;
  kinds(uri: string): Promise<KindResolution>;
  capabilities(): string[];
  invalidate(uri: string): void;
  dispose(): void;
}

interface SessionOptions {
  workspace: Record<string, string>;
  configYAML: string;
}
```

JS method names match Go method names. JS string
arguments map to Go `[]byte` at the boundary;
URIs stay strings on both sides. Diagnostic
range columns are UTF-16 code units (LSP
default); the Go side measures them once and
the WASM bridge passes them through unchanged.
Go methods that return `(T, error)` map to a
`Promise<T>` that rejects with `new Error(msg)`
on the Go error string. New Go methods appear
on the JS side in the same release.

### Open namespace and capabilities

`Session.Capabilities()` lists the methods
available on this session:

```go
session.Capabilities() // ["check", "fix", "kinds"]
```

JS callers feature-detect the same way:

```ts
if (session.capabilities().includes("extract")) {
  session.extract(uri, source);
}
```

The list contains method names — never rule
IDs. A WASM build that lacks (say) the future
`rename` method omits it here. Future plans add
`extract`, `query`, `deps`, `rename`, `hover`,
`completion` to both sides without rearranging
existing methods. Each new method declares
itself once on Go's `Session` and once on the
proxied JS session. No central registry.

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
  Cleared on workspace deltas via `Invalidate`.

The first `Check` parses; later `Check` and
`Fix` on the same source skip parse. Plan 217's
steady-state targets depend on this.

### Workspace abstraction

`pkg/mdsmith.Workspace` replaces every
`os.ReadFile` site in `internal/`. `OSWorkspace`
wraps `os.ReadFile` and `filepath.Glob` for the
native build. `MemWorkspace`, backed by
`map[string][]byte`, drives the WASM build and
native tests. At WASM session construction the
JS `workspace: Record<string, string>` map
becomes a `MemWorkspace` once;
`Invalidate(uri)` is the only path for callers
to flush a cached file.

`MemWorkspace.Glob` is a linear key filter; the
hot loop must not call it per file. A benchmark
fixture asserts the rule registry stays clear of
per-file `Glob` calls under `MemWorkspace`.

### MDS040 and build tag

`internal/rules/recipesafety` (MDS040) needs
real shell access. A `//go:build !wasm`
directive on its registration excludes it from
the rule registry under WASM. `check` runs the
remaining rules; a single notice in
`docs/background/concepts/engine-api.md` names
the rule as out of scope on WASM.

### Build and smoke test

`cmd/mdsmith-wasm/build.sh` runs the trimmed
`go build` against `GOOS=js GOARCH=wasm` and
copies `wasm_exec.js` from `$(go env GOROOT)`.
A second target uses `tinygo`. Both artifacts
land under `cmd/mdsmith-wasm/dist/` for plan
217.

The smoke test runs under
[`wasmbrowsertest`][wbt] or a Node loader. It
creates a session against
`{ "doc.md": "# Title\nbody" }`, calls
`session.check`, and asserts the result matches
the native CLI.

[wbt]: https://github.com/agnivade/wasmbrowsertest

### Bundle size

Standard Go WASM trimmed: ≤ 18 MB. `tinygo`:
≤ 8 MB. Build records both numbers in the
completion note.

### Docs

`docs/background/concepts/engine-api.md` covers
the session shape, the cache model, the
open-namespace pattern, and WASM constraints.

## Tasks

1. Doc-only commit sketching `pkg/mdsmith` —
   `Session`, `SessionOptions`, `Workspace`,
   the capability list, method signatures.
   Iterate before implementation.
2. Add `pkg/mdsmith.Workspace` with
   `OSWorkspace` and `MemWorkspace`. Refactor
   every `os.ReadFile` site in `internal/` to
   take a `Workspace`. Add a test against
   `MemWorkspace`.
3. Build the `Session` type with parse-AST and
   config caches. Each method is a thin shim
   over `internal/engine` and `internal/fix`.
4. Migrate `cmd/mdsmith/` and `internal/lsp/`
   to `NewSession`. The LSP server uses one
   session per workspace and invalidates on
   `didChange` / `didChangeWatchedFiles`.
5. Add `//go:build !wasm` on the recipesafety
   rule's `init`. Native unaffected.
6. Add `cmd/mdsmith-wasm/main.go`. Register
   `globalThis.mdsmith.createSession` and
   `globalThis.mdsmith.version`. Build with
   both `go` and `tinygo`; record the smaller
   correct artifact.
7. Smoke test: WASM `session.check` matches
   the native CLI on an in-memory fixture.
8. Write
   `docs/background/concepts/engine-api.md`.
9. Run `mdsmith fix .` and confirm
   `mdsmith check .` passes.

## Acceptance Criteria

- [ ] `pkg/mdsmith` exposes `Session`,
      `NewSession`, `Check`, `Fix`, `Kinds`,
      `Capabilities`, `Invalidate`, `Dispose`,
      plus `Workspace` with `OSWorkspace` and
      `MemWorkspace`. `cmd/mdsmith` and
      `internal/lsp` use `NewSession`; no
      `os.ReadFile` survives outside
      `pkg/mdsmith` and `cmd/`.
- [ ] `cmd/mdsmith-wasm/` builds with
      `GOOS=js GOARCH=wasm` and with `tinygo`,
      exporting `globalThis.mdsmith.createSession`
      and `globalThis.mdsmith.version`. A test
      asserts the JS session method set matches
      the Go `Session` method set name-for-name.
- [ ] `Capabilities()` returns method names
      (never rule IDs) and returns the same
      list in Go and JS for the same build.
- [ ] Repeated `Check(uri, source)` on the
      same source-hash reuses the parsed AST.
      Bench shows steady-state under half the
      cold-start time.
- [ ] Smoke test: WASM `check` matches native
      CLI on an in-memory fixture.
- [ ] Standard Go WASM ≤ 18 MB; `tinygo` ≤
      8 MB. Completion note records which ships.
- [ ] `docs/background/concepts/engine-api.md`
      exists. `mdsmith check .`, `go test ./...`,
      and `go tool golangci-lint run` all pass.

## Non-Goals

- The Obsidian plugin UI — see
  [plan 217](217_obsidian-plugin.md).
- New methods beyond `check`, `fix`, `kinds`.
  Open namespace lets future plans add
  `extract`, `query`, `deps`, `rename`, `hover`,
  `completion` without changing this plan.
- WASM builds for `npm`, `pip`, or other
  channels — the artifact targets in-process JS.
- Recipe execution under WASM.
- A standalone WASM playground on the website.

## See also

- [Plan 163: public Markdown library](163_public-markdown-library.md)
- [Plan 65: WASM spike](65_spike-wasm-embedded-inference.md)
- [Plan 217: Obsidian plugin](217_obsidian-plugin.md)
- [Plan 214: hand-rolled LSP bridge (⛔)](214_obsidian-plugin.md)
