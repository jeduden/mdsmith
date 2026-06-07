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

The core operations are thin shims over the engine and the fixer. Each
takes one URI and its in-memory bytes:

```go
func (s *Session) Check(uri string, source []byte) ([]Diagnostic, error)
func (s *Session) Fix(uri string, source []byte) (FixResult, error)
func (s *Session) Kinds(uri string) (KindResolution, error)
```

Two native-only batch operations lint or fix many files on disk in one
call. Each drives the engine's parallel path loop and returns its own
result type. So `cmd/mdsmith` keeps file discovery, ordering, and output
formatting, but drops its own engine plumbing:

```go
func (s *Session) CheckPaths(paths []string, opts BatchOptions) *engine.Result
func (s *Session) FixPaths(paths []string, opts BatchOptions) *fix.Result
```

`BatchOptions` carries the `--explain` flag, the parallel concurrency
cap (`GOMAXPROCS` by default), a per-call byte cap, a verbose logger,
and a dry-run flag for `FixPaths`. These two methods have **no
JavaScript mirror**: a browser host has no disk, no file walk, and no
write-back, so it lints single buffers through `Check` / `Fix` instead.
The `MemWorkspace` is the only filesystem under WASM. See [open
namespace and capabilities](#open-namespace-and-capabilities) for how
the native-only set stays in lock-step despite the gap.

Three more native-only methods serve the LSP server. `CheckVersion`
takes the editor's `textDocument` version, so the [version-keyed parse
cache](#caching) can serve a re-lint at the same version without
re-parsing. `FixRule` applies one rule's fixes for a quick-fix
lightbulb. `ResolveFile` returns the raw per-rule resolution the `kinds
why` command walks:

```go
func (s *Session) CheckVersion(uri string, source []byte, version int) *engine.Result
func (s *Session) FixRule(uri string, source []byte, names []string) (FixResult, error)
func (s *Session) ResolveFile(uri string, fmKinds []string, fmFields map[string]any) *config.FileResolution
```

`CheckVersion` returns the engine `Result`, not the public `Diagnostic`
slice. That keeps the LSP's own diagnostic partitioning and error
surfacing, consistent with the batch ops above.

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

A `Workspace` reads through two paths that must agree on which file a
URI names: `ReadFile` (which `Session.Kinds` uses to read front matter)
and the `fs.FS` view (which cross-file rules — catalog, include, links —
read through). `OSWorkspace` carries an optional `Root`; the CLI sets it
to the project root so workspace-relative URIs match config globs. With
a `Root`, `ReadFile` resolves a relative URI against it, exactly as the
`Root`-rooted `fs.FS` does, so the same URI cannot resolve to two
different files. An absolute path is read unchanged, and an empty `Root`
reads paths as passed. `MemWorkspace` keys both paths off the same map.

A third implementation, `OverlayWorkspace`, is the LSP server's. It
reads disk rooted at `Root` but lets `Set(uri, bytes)` shadow a path's
content with an editor's unsaved buffer, so cross-file rules read the
live buffer rather than the last saved file. Only content is overlaid —
open buffers still exist on disk, so globbing and directory walks defer
to disk, and the `fs.FS` view clones only the small open-buffer map per
lint pass, never the corpus. That keeps a per-keystroke `CheckVersion`
off any `O(corpus)` snapshot cost.

`MemWorkspace.Glob` is a linear key filter. The lint hot loop must not
call it per file; a benchmark fixture asserts no per-file `Glob` under
`MemWorkspace`.

The configuration arrives through a `ConfigSource`. `ConfigYAML` carries
inline YAML (the WASM path) and `ConfigPath` names a `.mdsmith.yml` on
disk; `NewSession` merges either over the built-in defaults.
`ConfigCompiled` wraps an already-merged `*config.Config`: the CLI
loads, merges, injects build recipes, and installs the include-extract
projector, then hands the result over so those side effects survive. A
compiled source is taken as-is.

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

`Capabilities()` advertises only the mirrored single-file surface
(`check`, `fix`, `kinds`). The native-only batch ops (`checkPaths`,
`fixPaths`) are left off, because a JS host can never call them. A
native test ties the JS proxy method set to the Go `Session` method set
minus an explicit native-only allowlist. A new *mirrored* method added
on one side but not the other fails the build. A new native-only batch
method is allowed once it joins that allowlist.

## Caching

The session owns four caches, all session-scoped:

- **Check results.** One entry per URI, holding the last
  `(content-hash, diagnostics)` pair. The next `Check` on the same URI
  with the same content reuses it without re-parsing or re-linting; a
  no-op `Fix` reuses it too.
- **Version-keyed parse.** One parsed document per URI, keyed by the
  editor's `textDocument` version. `CheckVersion` serves it, so a
  re-lint at the same version (a code action, a hover) skips the parse.
  An edit bumps the version and misses, forcing a re-parse. This is the
  per-keystroke cache the LSP latency gate depends on.
- **Cross-file read.** The engine read cache shared across operations,
  so a catalog or include target read by one buffer is not re-read by
  the next on every keystroke.
- **Compiled config.** Built once at `NewSession`. A config change
  needs `Dispose()` plus a new `NewSession`; there is no in-place
  reconfigure.

`Invalidate(uri)` signals that `uri` changed. With a `content` argument
it rewrites that file through the workspace's mutable overlay, so the
next cross-file `Check` reads the new bytes — this is how the LSP's
unsaved-buffer bytes reach catalog, include, and link rules. A bare
`OSWorkspace` has no overlay and re-reads disk; a no-`content` call on a
mutable workspace deletes the entry (file removed). Invalidate drops the
changed path's read-cache and version-parse entries, then drops every
cached `Check` result, not only `uri`'s. A changed file can feed any
other through a cross-file rule, and the session keeps no dependency
graph, so a stale dependent must never be served.

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
The engine build-tags a CUE-free path behind `//go:build wasm` to keep
the artifact small and let `tinygo` compile it. The CUE module (95
packages) and protobuf are dropped. `Session.Capabilities` still
returns `["check", "fix", "kinds"]`, so every method works. But some
rule-internal behaviours that need CUE or the OS filesystem degrade to
a no-op on WASM. These per-rule degradations are not in the capability
list, because they do not remove a method.

What the WASM build drops, and why:

- **MDS040 (recipe safety) is out of scope.** It shell-safety-checks
  build recipes and needs real shell access, so its registration sits
  behind `//go:build !wasm`. The package still compiles under WASM;
  the rule self-registers only on native.
- **CUE front-matter validation (MDS020) is skipped.** The
  heading-structure, filename, and content checks still run; only the
  `frontmatter:` CUE-constraint check (`internal/schema`'s
  CUE-backed `ValidateFrontmatter` / `validateFrontmatterDiags`) is a
  no-op. A document's front matter is not validated against its
  schema's CUE constraints on WASM.
- **Catalog `where:` queries and CUE row templates are inert.**
  `internal/query` (the `where:` matcher) and `internal/cuetemplate`
  (CUE `row:` expressions) return a "not available in the WASM build"
  error, which the catalog rule surfaces as a diagnostic on that
  directive. Plain `{field}` row templates still work — they run
  through the CUE-free `internal/fieldinterp` path, which reimplements
  the dotted/quoted path parser without CUE.
- **Kind `extends` skips the merge-time contradiction check.** The
  extends merge still runs; `internal/schema`'s `checkUnifiable` (which
  compiled the unified CUE constraint to detect `int & string`-style
  conflicts) is a no-op.
- **No disk reads or writes.** Cross-file rules read through the
  `MemWorkspace` the host supplies, never the OS filesystem. The
  `<?index?>` schema sidecar (`schema.WriteIndex` / `ValidateIndex`)
  and the git-hook / `.gitattributes` writers are no-ops — they have
  no OS disk to act on, and their `os.Chmod` / `os.SameFile` calls are
  among the standard-library functions `tinygo` omits.

The artifact lands under `cmd/mdsmith-wasm/dist/` for plan 217. The
build script (`cmd/mdsmith-wasm/build.sh`) copies `wasm_exec.js`
alongside the artifact. It reads it from `$(go env GOROOT)/lib/wasm/`
for standard Go, or `$(tinygo env TINYGOROOT)/targets/` for tinygo. A
smoke test creates a session, calls `session.check`, and asserts the
result matches the native engine on the same in-memory fixture. It runs
against both the standard-Go and the tinygo artifact.

### Size budget

Plan 218 brought both artifacts under the budgets plan 215 set:

- **Standard-Go WASM: ~10.5 MB raw (~2.7 MB gzipped, the figure that
  crosses the wire), under the 18 MB budget.** Removing CUE and
  protobuf via the `//go:build wasm` path above dropped it from ~38 MB.
- **tinygo WASM: ~3 MB, under the 8 MB budget.** Two further fixes
  unblocked tinygo: `internal/lint/runcache.go` no longer uses
  `sync.Map.CompareAndDelete` (which tinygo's standard library omits) —
  a mutex-guarded compare-and-delete replaces it — and the on-disk
  `os.Chmod` / `os.SameFile` call sites are build-tagged out. The
  tinygo build also needs `-stack-size=1MB`: the engine's package init
  (rule registry, the regexp tables in `internal/lint`, config
  defaults) overflows tinygo's default 64 KB goroutine stack, which
  otherwise traps at startup as "memory access out of bounds".

`cmd/mdsmith-wasm/size_test.go` asserts both budgets. The standard-Go
test runs on every host. The tinygo test runs only where the `tinygo`
toolchain is installed, and skips otherwise.

## See also

- [Plan 215: engine API and WASM bindings](../../../../plan/215_engine-api-wasm.md)
- [The public Markdown library](../../development/markdown-library.md)
- [How flavor, rule, convention, and kind differ](flavor-rule-convention-kind.md)
