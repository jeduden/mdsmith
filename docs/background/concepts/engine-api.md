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

`Session.Kinds` caps its front-matter read at the session's
`max-input-size`, the same cap `Check`/`Fix` already apply to the
engine's parse input. A file over the cap resolves the same lenient way
a missing file does: no error, and no front-matter-derived kinds.

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
An example is `["check", "fix", "kinds", "rename", "move"]`. JS mirrors
it as `session.capabilities()`. A caller feature-detects with
`capabilities().includes("move")` before calling a method.

The list holds method names, never rule IDs. Future plans add methods
such as `extract`, `query`, `deps`, `hover`, and `completion` to both
sides without rearranging the existing methods. Each new method declares
itself once on Go's `Session` and once on the proxied JS session — there
is no central registry.

`Capabilities()` advertises the mirrored single-file surface (`check`,
`fix`, `kinds`) plus the refactor pair (`rename`, `move`). Each returns
a neutral `RefactorPlan`: text edits keyed by workspace-relative file,
plus an optional `move`. The engine touches no files. The host applies
the edits and performs any move itself — a WASM host through its own
API, since `git mv` is unavailable under `GOOS=js`.

The native-only batch ops (`checkPaths`, `fixPaths`) are left off,
because a JS host can never call them. A native test ties the JS proxy
method set to the Go `Session` method set minus an explicit native-only
allowlist. A new *mirrored* method added on one side but not the other
fails the build. A new native-only batch method is allowed once it joins
that allowlist.

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
  rename(uri: string, src: string, as: string, oldName: string,
    newName: string): Promise<Plan>;
  move(src: string, dst: string): Promise<Plan>;
  capabilities(): string[];
  invalidate(uri: string, content?: string): void;
  dispose(): void;
}

interface SessionOptions {
  workspace?: Record<string, string>;
  configYAML?: string;
}
```

JS method names match the Go names. A JS string argument crosses as a
Go `[]byte` while a URI stays a string. The JS `configYAML` string
becomes a Go `ConfigSource` exactly as the `-c` flag's text does.
`createSession` returns a `Promise` because `WebAssembly.instantiate`
is async, and any Go method returning `(T, error)` maps to a
`Promise<T>` that rejects with `new Error(msg)`.

Some outcomes are not failures. A rename to the symbol's own text,
or to the text a heading already renders as, has no effect. In Go,
`Session.Rename` then returns an error that matches
`mdsmith.ErrNothingToRename` under `errors.Is`. In JS, the
rejected `Error` has `code` set to `"nothing-to-rename"`, the value of
`mdsmith.ErrorCode(err)`. A host can check the code and ignore the
no-op without reading the message. Other errors leave `code` unset.

No session registers a function of its own. The method functions
are shared by all sessions and registered once. Each method on a
session object is a `bind` of one of them with a session id, so the
binding is collected once that method is unreachable. A method taken
off the object, such as `const { check } = session`, keeps its
binding after the object is gone.

Each session also has a token object. `dispose` is bound to it, and a
private `WeakMap` maps every other method to it, so the token lives
while any method does and no call but `dispose()` carries it. A
`FinalizationRegistry` watches the token without keeping it alive.
Once the object and every method taken off it are collected, the
registry disposes the session's id, so a host that drops a session
without `dispose()` still frees the Go session. That is a fallback:
the Go and JS garbage collectors decide when it runs, so call
`dispose()` when you are done with a session to free its caches and
workspace at once.

Go holds the session's JS values until its own collector runs. On
WebAssembly that happens only as the Go heap grows, so an engine that
is no longer called can keep a dropped session for good. The TinyGo
build never releases a JS value that Go has held, so there the token
is never collected and only `dispose()` frees a session. A host with
no `FinalizationRegistry`, or one the engine cannot build or bind,
still loads the standard Go build. It has no fallback either, and only
`dispose()` frees a session.

`dispose()` drops the id and cancels the registry entry, so the
disposed session's caches and workspace can be freed, and a
create/dispose loop holds a fixed number of registered functions and
registry entries. Each method keeps its shape afterwards:
`check`, `fix`, `kinds`, `rename`, and `move` return a `Promise` that
rejects with `Error("session disposed")`.
`capabilities()` returns `[]`, and `invalidate()` and a second
`dispose()` do nothing.

These hold through every reference, because the shared function looks
the id up on each call. Three cases reach the same disposed path. One
is a method taken before `dispose()`, such as
`const { check } = session` or `const d = session.dispose`. Another is
a session object frozen with `Object.freeze(session)`. The last is a
method made read-only with
`Object.defineProperty(session, "check", { writable: false })`. None
of them logs "call to released function".

Each id is a counter passed through a 10-round Feistel permutation
whose round function is AES-128 under a key drawn at random when the
engine loads. The ids span 2^53 values in both the standard Go and
the TinyGo build. So a script that reaches a raw shared function
cannot find a session by trying 0, 1, 2, and so on, or by stepping
from an id it knows. Ids it sees after a `Reflect.apply` patch do not
predict the ids handed out before it, short of breaking AES. The
permutation never maps two counter values to the same id, so no id
is handed out twice. A method kept from a disposed session never
reaches a later one.

The engine also binds through a `bind` captured at load, so a later
patch of `Function.prototype.bind` or `call` never sees a raw shared
function. This is hardening, not a privilege boundary. A `bind` or
`call` patched before the engine loads sees each raw shared function
and the id of every session. `wasm_exec.js` looks up `Reflect.apply`
on every Go-to-JS call, so a patched `Reflect.apply` sees the same
for each session created while the patch is in place, and every
session object the engine resolves. Go also reads the arguments of
each call into a method through `Reflect.get`, the bound id first.
So a patched `Reflect.get` sees the id of each session whose method
is called while the patch is in place.

A random id or a token object crosses those calls too, so the engine
does not try to hide it. The keyed id shields only a session that is
neither created nor called while such a patch is in place.

An argument of the wrong type, a `BigInt` included, makes an async
method reject and `invalidate()` do nothing. So does an options object
that throws when `createSession` inspects it, such as a revoked
`Proxy`. In the standard Go build this does not stop the Go runtime,
so other sessions keep working. Two cases still stop it. One is a
getter or `Proxy` `get` trap on the options object that throws, because
`wasm_exec.js` does not catch an exception from a property read. The
other is a `BigInt` passed to the TinyGo build, because TinyGo does not
implement `recover()` on WebAssembly.

`createSession` rejects when `opts` is not a plain object. It also
rejects when `opts.workspace` is present but is not a plain object of
path-to-source strings: `null`, a string, an array, and a boxed
`String` all reject. An array-like is rejected because its indices
would otherwise become file paths `"0"`, `"1"`, and so on. An absent
`workspace` means an empty workspace, and a non-string entry value is
skipped. A present `configYAML` that is not a string, such as `null` or
a `Buffer`, rejects too; an absent one means the default config.

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

Both target budgets are met and CI-verified:

- The standard Go WASM artifact is about 13.3 MiB uncompressed (about
  3.3 MiB gzipped at standard compression, the figure that crosses the
  wire; `cmd/mdsmith-wasm/size_test.go` measures about 4.1 MiB at the
  pessimistic BestSpeed level and fails above 4.25 MiB). It was about
  40 MB before
  `cuelang.org/go` was removed: CUE (95 packages) plus `cockroachdb/apd`
  and protobuf were the dominant cost, pulled in by `internal/schema`
  (MDS020 file-schema validation), `internal/fieldinterp` (catalog and
  include field interpolation), and `internal/query`. The in-house
  `cue/cuelite` engine — a pure-Go, standard-library-only implementation
  of the exact CUE subset those packages use — replaced CUE, so the
  whole dependency graph left `go.mod` (plan 218/240). The artifact
  fits the ≤ 18 MB plan-215 budget with headroom to spare.
- `tinygo build -target wasm ./cmd/mdsmith-wasm` succeeds (plan 247).
  The `os.Chmod`, `os.SameFile`, and `os.Symlink`/`filepath.EvalSymlinks`
  calls that tinygo's wasm target does not implement are now behind
  build-tagged seams: no-ops or identity functions in the wasm sandbox
  where they are unreachable or unnecessary. The artifact fits the ≤ 8 MB
  plan-215/247 budget; `TestTinyGoWASMArtifactSizeBudget` enforces it in
  the `tinygo-wasm` CI job.

Both WASM builds are CI-verified and gate PRs.

## See also

- [Plan 215: engine API and WASM bindings](../../../../plan/215_engine-api-wasm.md)
- [The public Markdown library](../../development/markdown-library.md)
- [How flavor, rule, convention, and kind differ](flavor-rule-convention-kind.md)
