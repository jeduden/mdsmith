---
id: 217
title: Obsidian plugin (WASM runtime)
status: "🔲"
model: opus
summary: >-
  Ship an Obsidian plugin under
  `editors/obsidian/` that loads the WASM build
  from plan 215 and renders mdsmith diagnostics,
  quick-fixes, and fix-on-save inside
  CodeMirror 6. One runtime, one code path —
  desktop and mobile both use WASM.
depends-on: [215, 168]
---
# Obsidian plugin (WASM runtime)

## Goal

Surface mdsmith inside Obsidian on every
platform the editor supports — desktop,
iPadOS, iOS, Android. A writer opening a
`.md` file sees inline squiggles. A "Fix"
link applies the matching quick-fix.
Fix-on-save runs `fixAll` on the active
buffer.

[Plan 215](215_engine-api-wasm.md) provides
the lint engine — `pkg/mdsmith.Session` in Go,
mirrored as `globalThis.mdsmith.createSession`
in WASM. This plan ships the plugin shell that
hosts a session.

## Background

[Plan 168](168_obsidian-markdown-support.md)
added the `obsidian` convention. The gap
is the editor wiring.

Obsidian runs on Electron on desktop and a
sandboxed WebView on mobile. The sandbox
blocks subprocess spawning, native binary
loading, and direct filesystem access
through anything but the Vault API. WASM
sidesteps all three.

[Plan 214](214_obsidian-plugin.md) was an
earlier draft that spawned `mdsmith lsp` as a
subprocess. Plans 215 and 217 replace it.
Salvageable code from that branch
(`diagnostics.ts`, settings tab, code actions,
styles, build shell) may be cherry-picked.

## Design

### Runtime

`editors/obsidian/src/wasm-runtime.ts`
instantiates the WASM module from plan 215 and
holds a single
[`mdsmith.Session`](215_engine-api-wasm.md#wasm-bindings-cmdmdsmith-wasm)
for the vault:

```ts
const session = await mdsmith.createSession({
  workspace: workspaceSnapshot.toRecord(),
  configYAML: await loadConfig(),
});

// Per-file operations call methods on the session
await session.check(uri, source);
await session.fix(uri, source);
```

The wrapper exposes `check`, `fix`, `invalidate`,
and `dispose` through a thin typed facade so the
rest of the plugin never imports the WASM module
directly. On unload it calls `session.dispose()`.

`workspace.ts` keeps the same flat
`Map<string, string>` snapshot. The wrapper
materializes it into the session at construction.
Because the session's `MemWorkspace` is built
once, a vault edit must push the new bytes: on
`'modify'` / `'create'` the wrapper calls
`session.invalidate(uri, content)`, and on
`'delete'` it calls `session.invalidate(uri)`
with no content to drop the file. Each fan-out is
debounced 200 ms.

### Diagnostics in CodeMirror 6

`diagnostics.ts` uses [CodeMirror 6][cm6].
It adds a `StateField` per editor. The
field tracks the diagnostics. Each shows as
a wavy underline. Severity classes live in
`styles.css`.

A hover tooltip is issue-first (plan 230): the
message leads, then the schema constraint from
`related_locations` (a navigable link when it has
a file/line, else plain text), then the rule code
and a docs link. The "Fix" link runs the same flow
as the palette command.

A "mdsmith Diagnostics" [`ItemView`][iv]
lists every workspace diagnostic in a sortable
table; click jumps to source.

[cm6]: https://codemirror.net/
[iv]: https://docs.obsidian.md/Plugins/User+interface/Views

### Actions and fix-on-save

Three command surfaces:

1. The hover tooltip "Fix" link.
2. Per-line palette commands. Each active
   diagnostic on the cursor line registers
   a transient `mdsmith: Fix — {code}`
   command. The set clears on cursor move.
3. The `mdsmith: Fix file` command awaits
   `session.fix(uri, source)` and applies the
   returned edits.

`fixOnSave` (off by default) debounces
`vault.on('modify')` and triggers
`Fix file` 200 ms after the last save.

### Settings

| Setting      | Default    | Purpose                 |
| ------------ | ---------- | ----------------------- |
| `configPath` | `""`       | Override `.mdsmith.yml` |
| `runMode`    | `"onSave"` | `onType`/`onSave`/`off` |
| `fixOnSave`  | `false`    | Run `fixAll` after save |

Settings round-trip via `loadData` and
`saveData`. Changing `configPath` calls
`session.dispose()` and creates a fresh session
with the new config; plan 215 does not expose
an in-place reconfigure.

### Lifecycle

`onload` runs in order. Read settings. Load the
WASM bundle. Build the workspace snapshot. Call
`mdsmith.createSession({workspace, configYAML})`
once. Register the CM6 extension, the commands,
the diagnostics view, and the vault listeners.

`onunload` calls `session.dispose()`, removes
the listeners, and clears the views. A
`mdsmith: Restart session` command runs the
same `dispose + createSession` flow used on
config changes.

### Budgets

- Cold-start `check` on a 1000-line file:
  ≤ 1 s desktop, ≤ 2 s on a modern iPad.
- Steady-state `check`: ≤ 150 ms everywhere.
- Release zip ≤ 25 MB. If WASM pushes past
  25 MB, fetch it via [`requestUrl`][ru] on
  first run and cache the bytes.

Benchmarks come from `wasm-runtime.bench.ts`.

[ru]: https://docs.obsidian.md/Reference/TypeScript+API/requestUrl

### Build and release

`bun run build.ts --production` bundles
the TS, copies the WASM artifact from
plan 215, and zips `dist/`. CI runs
`bun test`, then the build, then attaches
the zip to the GitHub Release. The
release pipeline picks it up the same way
it picks up the `.vsix`.

`manifest.json` does NOT set
`isDesktopOnly`.

### Docs

A new guide page covers install, settings,
and common issues. Three other pages get a
one-line note. The [linter comparison][lc]
cites the plugin. The
[conventions reference][conv] pairs
`convention: obsidian` with it. The
[github-releases page][gh] lists the
artifact.

[lc]: ../docs/background/markdown-linters.md
[conv]: ../docs/reference/conventions.md
[gh]: ../docs/development/release-channels/github-releases.md

## Tasks

1. Scaffold `editors/obsidian/`:
   `package.json`, `tsconfig.json`,
   `manifest.json` (no `isDesktopOnly`),
   `build.ts`, stub `src/main.ts`,
   `README.md`.
2. Implement `wasm-runtime.ts`. Instantiate
   the plan-215 WASM via
   `WebAssembly.instantiate`. Construct one
   `mdsmith.Session` per vault from the
   workspace snapshot and the config YAML.
   Expose `session.check`, `session.fix`,
   `session.invalidate`, and `session.dispose`
   through a typed facade. Cover with `bun test`.
3. Implement `workspace.ts`. Snapshot
   `app.vault.getMarkdownFiles()`. On `'modify'`
   / `'create'` fire
   `session.invalidate(uri, content)`; on
   `'delete'` fire `session.invalidate(uri)`.
   Debounce each fan-out 200 ms.
4. Implement `diagnostics.ts`. Add the
   CM6 `StateField`, the effect type, and
   a `hoverTooltip` provider rendering
   code, message, and a Fix link.
5. Implement `actions.ts`. Add per-line
   palette commands from active
   diagnostics, the `Fix file` command,
   and the debounced `vault.on('modify')`
   handler.
6. Implement `settings.ts`. Wire the
   three controls, the
   `loadData`/`saveData` round-trip, and
   a runtime restart when `configPath`
   changes.
7. Wire `main.ts`. Register the CM6
   extension, the diagnostics view, the
   commands, and the settings tab.
8. Add `styles.css` for severity
   underlines and tooltip styling.
9. Add `wasm-runtime.bench.ts`.
   Benchmark cold start and steady state
   on a 1000-line fixture.
10. Add a `.github/workflows/` step that
    builds the plugin and uploads the
    zip as a release artifact.
11. Write `docs/guides/editors/obsidian.md`.
    Update the conventions reference,
    the linter-comparison page, and the
    GitHub Releases page.
12. Run `mdsmith fix .` and confirm
    `mdsmith check .` passes.

## Acceptance Criteria

- [ ] `editors/obsidian/` builds with
      `bun run build.ts --production`.
      Output: `dist/main.js`,
      `dist/mdsmith.wasm`,
      `dist/wasm_exec.js`,
      `manifest.json`, `styles.css`.
- [ ] `bun test` passes. Coverage spans
      runtime marshalling, workspace
      snapshot, diagnostics decoration,
      and settings round-trip.
- [ ] Loading the plugin in a vault with
      an `MDS001` violation shows a wavy
      underline within 1 s of opening the
      file on desktop, 2 s on a modern
      iPad.
- [ ] Hover tooltip is issue-first: message,
      then the schema constraint (a navigable link
      when it has a file/line, else plain text),
      then rule code and a docs link. The "Fix"
      link applies the quick-fix.
- [ ] `mdsmith: Fix file` produces the
      same buffer as `mdsmith fix` on the
      same input.
- [ ] `fixOnSave: true` runs `Fix file`
      after each save without a plugin
      restart.
- [ ] `manifest.json` does NOT set
      `isDesktopOnly`. Mobile loads the
      plugin.
- [ ] Release zip stays under 25 MB.
- [ ] Cold-start `check` on the
      1000-line fixture: ≤ 1 s on
      desktop, ≤ 2 s on a modern iPad.
      Steady-state: ≤ 150 ms.
- [ ] CI attaches
      `mdsmith-obsidian-<version>.zip` to
      the release artifacts.
- [ ] `docs/guides/editors/obsidian.md`
      exists. The linter-comparison page
      cites the new plugin.
- [ ] `mdsmith check .` passes.

## Non-Goals

- LSP, JSON-RPC, or subprocess spawning.
- A WASM build for `npm`, `pip`, or other
  channels.
- LSP hover, completion, rename, and
  symbol navigation. Each goes in its
  own follow-up.
- Submission to the Obsidian Community
  Plugins catalog. The channel is GitHub
  Releases.
- Live-preview rendering changes.
- New rule bindings beyond what the
  `obsidian` convention activates.

## See also

- [Plan 215: engine API and WASM bindings](215_engine-api-wasm.md)
- [Plan 168: Obsidian convention](168_obsidian-markdown-support.md)
- [Plan 214: hand-rolled LSP bridge (⛔)](214_obsidian-plugin.md)
