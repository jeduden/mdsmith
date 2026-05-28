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

Give every consumer — the CLI, the LSP server,
the Obsidian plugin from
[plan 217](217_obsidian-plugin.md), future
website playgrounds — one Go API for the lint
engine. Mirror it in JavaScript via WebAssembly
so JS hosts call the same method names with the
same shapes.

## Background

mdsmith's engine lives under `internal/` today.
`cmd/mdsmith/` and `internal/lsp/` each re-derive
their own plumbing — load config, compile rules,
walk files. No public Go surface mirrors in JS.
[Plan 163](163_public-markdown-library.md)
extracted `pkg/markdown`. This plan extends the
same pattern to the engine.

WebAssembly under `GOOS=js GOARCH=wasm` has no
filesystem, no subprocess, no threads. The 10 MB
Go runtime is the size cost. `tinygo` trims to
5–8 MB; the build target builds both and the
completion note records which ships.

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
arguments map to Go `[]byte`; URIs stay strings
on both sides. New Go methods appear on the JS
side in the same release.

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

Each build advertises what it supports. Native
includes `mds040`; WASM does not. Future plans
add `extract`, `query`, `deps`, `rename`,
`hover`, `completion` to both sides without
rearranging existing methods. Each new method
declares itself once in Go (a method on
`Session`) and once in JS (a property on the
proxied session). No central registry.

### Caching

The session owns three caches, all
session-scoped:

- Parsed AST (`pkg/markdown.Document`), keyed by
  URI + content hash. Cleared by
  `Invalidate(uri)`.
- Compiled config, built once at `NewSession`
  and rebuilt on the explicit restart hook.
- Workspace `ReadFile` results, keyed by path.
  Cleared on workspace deltas via `Invalidate`.

The first `Check` parses; later `Check` and
`Fix` calls on the same source skip parse. Plan
217's steady-state targets depend on this.

### Workspace abstraction

`pkg/mdsmith.Workspace` replaces every
`os.ReadFile` site in `internal/`. Two
implementations:

- `OSWorkspace` — wraps `os.ReadFile` and
  `filepath.Glob`. Native build.
- `MemWorkspace` — backed by
  `map[string][]byte`. WASM build, and native
  tests.

`MemWorkspace.Glob` is a linear key filter; the
hot loop must not call it per file.

### MDS040 and build tag

MDS040 (recipe shell scanning) needs real shell
access. A build tag gates it out of the WASM
build, and `Capabilities()` omits `mds040` so
callers surface a notice instead of silently
dropping diagnostics.

### Build and smoke test

`cmd/mdsmith-wasm/build.sh` runs `go build
-trimpath -ldflags="-s -w"` against
`GOOS=js GOARCH=wasm` and copies `wasm_exec.js`
from `$(go env GOROOT)/lib/wasm/`. A second
target builds with `tinygo`. Both artifacts
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

- Standard Go WASM trimmed: ≤ 18 MB.
- `tinygo` WASM trimmed: ≤ 8 MB.

Build records both numbers in the completion
note.

### Docs

A new background page covers `pkg/mdsmith`.
Topics: the session shape. The cache model. The
open-namespace pattern. The WASM constraints.

## Tasks

1. Doc-only commit sketching the `pkg/mdsmith`
   surface: `Session`, `SessionOptions`,
   `Workspace`, the capability list, the method
   signatures. Iterate before implementation.
2. Introduce `pkg/mdsmith.Workspace` with
   `OSWorkspace` and `MemWorkspace`. Refactor
   every `os.ReadFile` site in `internal/` to
   take a `Workspace`. Tests stay green; one
   new test runs the same path against
   `MemWorkspace`.
3. Build the `Session` type and wire the
   parse-AST and config caches. Each method is
   a thin shim over `internal/engine` and
   `internal/fix`.
4. Refactor `cmd/mdsmith/` and `internal/lsp/`
   to use `NewSession`. The LSP server reuses
   one session per workspace and invalidates
   on `didChange` and `didChangeWatchedFiles`.
5. Add the build tag that gates MDS040 out of
   the WASM build.
6. Add `cmd/mdsmith-wasm/main.go`. Register
   `globalThis.mdsmith.createSession` and
   `globalThis.mdsmith.version`.
7. Write the build target. Produce both the
   standard Go and `tinygo` artifacts. Record
   the smaller correct one.
8. Smoke test: WASM `session.check` matches
   native CLI for an in-memory fixture.
9. Write
   `docs/background/concepts/engine-api.md`.
10. Run `mdsmith fix .` and confirm
    `mdsmith check .` passes.

## Acceptance Criteria

- [ ] `pkg/mdsmith` exposes `Session`,
      `NewSession`, `Check`, `Fix`, `Kinds`,
      `Capabilities`, `Invalidate`, `Dispose`,
      plus `Workspace` with `OSWorkspace` and
      `MemWorkspace`.
- [ ] `cmd/mdsmith` and `internal/lsp` use
      `mdsmith.NewSession`. No `os.ReadFile`
      survives outside `pkg/mdsmith` and
      `cmd/`.
- [ ] `cmd/mdsmith-wasm/` builds with
      `GOOS=js GOARCH=wasm` and with `tinygo`.
      The artifact exports
      `globalThis.mdsmith.createSession` and
      `globalThis.mdsmith.version`.
- [ ] The JS session method set matches the Go
      `Session` method set name-for-name. A
      test asserts both.
- [ ] `Capabilities()` returns the same list
      in Go and JS for the same build. Native
      lists `mds040`; WASM does not.
- [ ] Repeated `Check(uri, source)` on the
      same source-hash reuses the parsed AST.
      Bench shows steady-state under half the
      cold-start time.
- [ ] Smoke test: WASM `check` matches native
      CLI on an in-memory fixture.
- [ ] Standard Go WASM trimmed ≤ 18 MB;
      `tinygo` WASM trimmed ≤ 8 MB. Completion
      note records which toolchain ships.
- [ ] `docs/background/concepts/engine-api.md`
      exists.
- [ ] `mdsmith check .`, `go test ./...`, and
      `go tool golangci-lint run` all pass.

## Non-Goals

- The Obsidian plugin UI — see
  [plan 217](217_obsidian-plugin.md).
- New methods beyond `check`, `fix`, `kinds`.
  Open namespace lets future plans add
  `extract`, `query`, `deps`, `rename`,
  `hover`, `completion` without changing this
  plan.
- WASM builds for `npm`, `pip`, or other
  channels. The artifact targets in-process JS
  hosts.
- Recipe execution under WASM.
- A standalone WASM playground on the website.

## See also

- [Plan 163: public Markdown library](163_public-markdown-library.md)
- [Plan 65: WASM spike](65_spike-wasm-embedded-inference.md)
- [Plan 217: Obsidian plugin](217_obsidian-plugin.md)
- [Plan 214: hand-rolled LSP bridge (⛔)](214_obsidian-plugin.md)
