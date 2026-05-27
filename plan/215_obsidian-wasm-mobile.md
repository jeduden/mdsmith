---
id: 215
title: WASM mdsmith for mobile Obsidian
status: "🔲"
model: opus
summary: >-
  Build a `js+wasm` mdsmith target, expose a small
  `check`/`fix` JS API, and load it from the Obsidian
  plugin so the lint runs on iOS and Android where
  Electron's `child_process` is unavailable.
depends-on: [214]
---
# WASM mdsmith for mobile Obsidian

## Goal

Run mdsmith inside Obsidian on iOS and Android. A
user sees the same diagnostics they get on desktop.
The plugin loads a WASM build, runs `check` and
`fix` in process, and skips the binary spawn.

## Background

[Plan 214](214_obsidian-plugin.md) ships the
desktop bridge. It spawns `mdsmith lsp` via Node.
Mobile Obsidian has no Node. It also blocks native
binary loading from a plugin sandbox. The plan
marks itself `isDesktopOnly: true` and names this
follow-up as the mobile route.

Go compiles to WebAssembly with `GOOS=js
GOARCH=wasm`. The
[plan 65 WASM spike](65_spike-wasm-embedded-inference.md)
showed the build path works. The cost is binary
size (Go's runtime adds about 10 MB even with
trimming) and the loss of `os.Exec`, raw threads,
and stdio.

LSP fits poorly inside WASM. Client and server end
up in one process. The JSON-RPC framing buys
nothing. The plan exposes mdsmith as a small JS
API instead.

## Design

### Build target

A new `cmd/mdsmith-wasm/` builds with `GOOS=js
GOARCH=wasm`. The existing `cmd/mdsmith` keeps the
native build. The entry point exports an API
surface via `syscall/js`:

```go
js.Global().Set("mdsmith", js.ValueOf(map[string]any{
    "check":   js.FuncOf(check),
    "fix":     js.FuncOf(fix),
    "kinds":   js.FuncOf(kinds),
    "version": Version,
}))
```

`check(uri, source, configYAML)` returns a JS
array of diagnostics shaped like the LSP payload.
`fix(uri, source, configYAML)` returns the
rewritten file plus the edit list.
`kinds(configYAML)` mirrors the CLI.

The bundle ships at
`editors/obsidian/dist/mdsmith.wasm` next to
`main.js`. `wasm_exec.js` is the Go loader.

### Workspace abstraction

mdsmith reads files through `os.ReadFile`. Mobile
Obsidian routes file access through the [Vault
API][va]. The host filesystem is not available.

[va]: https://docs.obsidian.md/Reference/TypeScript+API/Vault

The plugin pre-resolves content on the JS side. It
passes a `Record<string, string>` of `relPath` to
content as a third argument. Inside Go, an
injectable `Workspace.ReadFile(path)` interface
replaces the direct `os.ReadFile` call. The native
build keeps using disk. The WASM build reads from
the injected map.

This refactor is the riskier piece. Task 1 lands
it before the WASM build does.

### Plugin glue

A new `editors/obsidian/src/wasm-runtime.ts`
mirrors `lsp-client.ts`:

```ts
export interface WasmRuntime {
  check(uri: string, source: string,
        workspace: Record<string, string>):
    Promise<Diagnostic[]>;
  fix(uri: string, source: string,
      workspace: Record<string, string>):
    Promise<{ output: string;
              edits: TextEdit[] }>;
  shutdown(): Promise<void>;
}
```

`main.ts` picks the runtime at load via
[`Platform.isMobile`][plat]:

```ts
const runtime = Platform.isMobile
  ? await loadWasmRuntime(ctx)
  : await spawnLspBridge(ctx);
```

[plat]: https://docs.obsidian.md/Reference/TypeScript+API/Platform

The `manifest.json` drops `isDesktopOnly: true`.
On desktop the LSP bridge stays the default. WASM
is opt-in via `mdsmith.preferWasm`. Native is
faster.

### Size and speed

Two budgets gate the plan:

- **Bundle.** The zip must stay under 25 MB. A
  trimmed Go WASM lands near 12–18 MB. A `tinygo`
  build lands near 5–8 MB. Task 3 builds both and
  picks one.
- **Cold start.** First `check` on a 1000-line
  file must finish in under 1 s on a modern iPad.
  Later calls must finish in under 150 ms.
  Numbers come from `wasm-runtime.bench.ts`.

If the bundle exceeds the cap, fetch the WASM via
[`requestUrl`][ru] on first run. The plugin
caches the bytes. Stretch goal — the default plan
in-bundles.

[ru]: https://docs.obsidian.md/Reference/TypeScript+API/requestUrl

### Rule subset

Some rules touch the host filesystem. Examples
include embed lint via `<?include?>`, recipe shell
scanning, and `<?catalog?>` glob expansion. Each
of these must run against the injected workspace
map. Some need to degrade. Task 1 enumerates the
list. Each rule gets a test against an in-memory
backing.

MDS040 needs real shell access. The WASM runtime
skips it. The diagnostics view shows a one-line
notice on mobile.

### Distribution

The `mdsmith-obsidian-<version>.zip` from plan 214
gains the WASM bytes. The release job grows a
step that builds the WASM target and stages it
under `editors/obsidian/dist/`. No new channel.

### Docs

The guide drops the "desktop only" caveat and
documents the `preferWasm` setting. A new
`docs/background/concepts/wasm-build.md` explains
the split: WASM vs native, what WASM can not do,
the size cap. The linter-comparison page picks up
mobile parity.

## Tasks

1. Refactor file reads in `internal/` behind a
   `Workspace.ReadFile(path)` interface. Each site
   keeps its tests green. One new test injects a
   memory-backed workspace.
2. Add `cmd/mdsmith-wasm/main.go`. Register
   `globalThis.mdsmith` with `check`, `fix`,
   `kinds`, and `version`. Run the binary under
   [`wasmbrowsertest`][wbt] or a Node loader to
   smoke-test the exports.
3. Extend `editors/obsidian/build.ts` with a
   `--wasm` flag. Build both with standard Go and
   `tinygo`. Pick one by size and correctness.
4. Implement `editors/obsidian/src/wasm-runtime.ts`.
   Instantiate via `WebAssembly.instantiate`.
   Marshal workspace snapshots in and diagnostics
   out. Test in `bun test` against the built WASM.
5. Wire runtime selection in
   `editors/obsidian/src/main.ts`. Pick WASM on
   mobile or when `preferWasm` is set. Add the
   `preferWasm` toggle to the settings tab.
6. Build a workspace snapshotter. Walk
   `app.vault.getMarkdownFiles()` at startup. Keep
   a `Map<string, string>`. Update on `'modify'`,
   `'create'`, and `'delete'` with a 200 ms
   debounce.
7. Add `wasm-runtime.bench.ts`. Benchmark cold
   start and steady state on a 1000-line fixture.
   Record results in the plan completion note.
8. Drop `isDesktopOnly: true` from the manifest
   once mobile smoke tests pass.
9. Write `docs/background/concepts/wasm-build.md`
   and update the Obsidian guide. Flip the
   Obsidian row in the linter-comparison page.
10. Run `mdsmith fix .` and confirm `mdsmith
    check .` passes.

[wbt]: https://github.com/agnivade/wasmbrowsertest

## Acceptance Criteria

- [ ] `cmd/mdsmith-wasm/` builds with `GOOS=js
      GOARCH=wasm`. It exports
      `globalThis.mdsmith.{check,fix,kinds,version}`.
- [ ] `mdsmith.wasm` and `wasm_exec.js` ship
      inside the obsidian release zip.
- [ ] Loading the plugin on Obsidian iOS lints a
      `.md` file with an `MDS001` violation. The
      underline shows within 2 s.
- [ ] Quick-fix on a mobile diagnostic produces
      the same buffer as `mdsmith fix` on desktop.
- [ ] Total packaged zip stays under 25 MB.
- [ ] Cold-start `check` on the 1000-line fixture
      finishes in under 1 s on a modern iPad.
      Later calls finish in under 150 ms.
- [ ] The manifest no longer has `isDesktopOnly`.
      Desktop keeps LSP as the default; WASM is
      opt-in via `preferWasm`.
- [ ] Every rule test passes against both the
      OS-backed and in-memory `Workspace`
      backings.
- [ ] MDS040 skips silently on the WASM runtime
      with a one-line notice in the diagnostics
      view.
- [ ] `docs/background/concepts/wasm-build.md` and
      the updated Obsidian guide ship.
- [ ] `mdsmith check .` passes; `go test ./...`
      passes; `go tool golangci-lint run` is
      clean.

## Non-Goals

- LSP over WASM. The JS API replaces the JSON-RPC
  bridge on mobile.
- A WASM build for `npm`, `pip`, or other
  channels. The artifact stays inside the
  Obsidian plugin.
- Recipe execution. Mobile has no shell.
- Indexing vaults larger than 10k Markdown files.
  Snapshot semantics are a linear walk. Larger
  vaults wait for a future plan.
- A standalone WASM playground on the website.

## See also

- [Plan 214: Obsidian plugin][214]
- [Plan 65: WASM spike][65]

[214]: 214_obsidian-plugin.md
[65]: 65_spike-wasm-embedded-inference.md
