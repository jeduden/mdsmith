---
summary: >-
  The public `pkg/mdsmith` engine API — a `Session` that owns
  workspace, compiled config, and parse caches — and how it mirrors
  one-to-one as WebAssembly JavaScript bindings, including the open
  method namespace, the cache contract, and the WASM size budgets and
  limits.
---
# Engine API and WASM bindings

`pkg/mdsmith` is the one public Go surface for the mdsmith engine. The
CLI, the LSP server, and the WebAssembly entry point all construct a
`Session` through it rather than re-deriving their own plumbing. The
same surface mirrors into JavaScript via WebAssembly with matching
method names and shapes, so a host such as the Obsidian plugin from
[plan 217](../../../../plan/217_obsidian-plugin.md) consumes one
contract whether it runs native or in a browser.

This page explains the mental model: what a `Session` owns, how the
open method namespace grows, what the three caches guarantee, and which
limits and size budgets apply under WebAssembly. It is a "why" page —
the exact signatures live in the package's Go doc comments.

## Session

A `Session` owns three things for one workspace. The first is the
[`Workspace`](#workspace), a filesystem abstraction. The second is the
compiled rule configuration. The third is the per-session
[caches](#caching). Reuse one session across many operations on the
same workspace. Construct it once with `NewSession`.

```go
package mdsmith

type Session struct{ /* ... */ }

func NewSession(opts SessionOptions) (*Session, error)

type SessionOptions struct {
    Workspace Workspace    // disk or in-memory
    Config    ConfigSource // path or inline YAML
}
```

The core operations are thin shims over the engine and the fixer:

```go
func (s *Session) Check(uri string, source []byte) ([]Diagnostic, error)
func (s *Session) Fix(uri string, source []byte) (FixResult, error)
func (s *Session) Kinds(uri string) (KindResolution, error)
```

Introspection and lifecycle round out the surface:

```go
func (s *Session) Capabilities() []string
func (s *Session) Invalidate(uri string, content ...[]byte)
func (s *Session) Dispose()
```

`Diagnostic`, `FixResult`, and `KindResolution` reuse the JSON shapes
the LSP server and the `--format json` CLI already emit, so a JS host
decodes them without a second schema. `Diagnostic` columns are UTF-16
code units (the LSP default), measured once on the Go side.

## Workspace

`Workspace` is the filesystem seam the engine reads through, so the
same engine code runs against a real disk and against an in-memory map:

```go
type Workspace interface {
    ReadFile(path string) ([]byte, error)
    Glob(pattern string) ([]string, error)
}
```

Two implementations ship. `OSWorkspace` wraps `os.ReadFile` and
`filepath.Glob` for native callers. `MemWorkspace`, backed by a
`map[string][]byte`, drives WebAssembly and native tests. There is no
disk under `GOOS=js GOARCH=wasm`. So the JS `workspace` map becomes a
`MemWorkspace` once at session construction. It then mutates only
through `Invalidate(uri, content)`.

`MemWorkspace.Glob` is a linear key filter. The lint hot loop must not
call it per file; a benchmark fixture asserts no per-file `Glob` under
`MemWorkspace`.

## Open namespace and capabilities

`Session.Capabilities()` returns the method names this build supports.
An example is `["check", "fix", "kinds"]`. JS mirrors it as
`session.capabilities()`. A caller feature-detects with
`capabilities().includes("extract")` before calling a method.

The list holds method names, never rule IDs. Future plans add methods
such as `extract`, `query`, `deps`, `rename`, `hover`, and `completion`
to both sides without rearranging the existing methods. Each new method
declares itself once on Go's `Session` and once on the proxied JS
session — there is no central registry.

## Caching

The session owns three caches, all session-scoped:

- **Parsed AST.** One entry per URI, holding the last
  `(content-hash, document)` pair. The next `Check` on the same URI
  with the same content reuses it. The first `Check` parses; later
  `Check` and `Fix` on the same source skip the parse.
- **Compiled config.** Built once at `NewSession`. A config change
  needs `Dispose()` plus a new `NewSession`; there is no in-place
  reconfigure.
- **Workspace `ReadFile` results.** Keyed by path.

`Invalidate(uri)` drops the cached parse and `ReadFile` bytes for
`uri`. With a `content` argument it also rewrites that file in a
`MemWorkspace` before flushing, so the next cross-file `Check` reads
the new bytes — without it, an edit to file B leaves file A's view of B
stale. `OSWorkspace` ignores `content` and re-reads disk; a
no-`content` call on a `MemWorkspace` deletes the entry (file removed).

## WASM bindings

`cmd/mdsmith-wasm/main.go` registers a JS factory that mirrors
`NewSession`:

```ts
declare const mdsmith: {
  createSession(opts: SessionOptions): Promise<Session>;
  version: string;
};

interface Session {
  check(uri: string, src: string): Promise<Diagnostic[]>;
  fix(uri: string, src: string): Promise<FixResult>;
  kinds(uri: string): Promise<KindResolution>;
  capabilities(): string[];
  invalidate(uri: string, content?: string): void;
  dispose(): void;
}

interface SessionOptions {
  workspace: Record<string, string>;
  configYAML: string;
}
```

JS method names match the Go names. A JS string argument crosses as a
Go `[]byte` while a URI stays a string. The JS `configYAML` string
becomes a Go `ConfigSource` exactly as the `-c` flag's text does.
`createSession` returns a `Promise` because `WebAssembly.instantiate`
is async, and any Go method returning `(T, error)` maps to a
`Promise<T>` that rejects with `new Error(msg)`.

## WASM limits and size budgets

`GOOS=js GOARCH=wasm` has no filesystem, no subprocess, and no threads.
Two consequences follow:

- **MDS040 (recipe safety) is out of scope on WASM.** That rule
  shell-safety-checks build recipes and needs real shell access, so its
  registration sits behind a `//go:build !wasm` tag. The package still
  compiles under WASM; the rule self-registers only on native. Every
  other rule runs.
- **No disk reads.** Cross-file rules read through the `MemWorkspace`
  the host supplies, never the OS filesystem.

The artifact lands under `cmd/mdsmith-wasm/dist/` for plan 217. The
build script (`cmd/mdsmith-wasm/build.sh`) copies `wasm_exec.js` from
`$(go env GOROOT)/lib/wasm/` alongside the standard Go artifact. A
smoke test creates a session, calls `session.check`, and asserts the
result matches the native engine on the same in-memory fixture.

### Size budget

The target budgets were a standard-Go artifact of ≤ 18 MB and a
`tinygo` artifact of ≤ 8 MB. The engine's actual dependency graph makes
neither reachable today:

- The standard Go WASM artifact is about 40 MB uncompressed (8.6 MB
  gzipped, the figure that crosses the wire). The dominant cost is CUE
  (95 packages) plus protobuf, pulled in by core engine packages —
  `internal/schema` (MDS020 file-schema validation), `internal/fieldinterp`
  (catalog and include field interpolation), and `internal/query`. CUE
  cannot be build-tagged out without disabling those features.
- `tinygo` does not compile the engine. The first wall is
  `sync.Map.CompareAndDelete` in `internal/lint/runcache.go`, which
  `tinygo`'s standard library omits, and CUE's heavy reflection would
  block it further.

So the shipping artifact is the standard Go WASM build. Bringing it
under budget — a CUE-free schema/field-interpolation path, or a
`tinygo`-compatible engine — is follow-up work tracked separately, not
part of this plan.

## See also

- [Plan 215: engine API and WASM bindings](../../../../plan/215_engine-api-wasm.md)
- [The public Markdown library](../../development/markdown-library.md)
- [How flavor, rule, convention, and kind differ](flavor-rule-convention-kind.md)
