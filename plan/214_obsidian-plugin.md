---
id: 214
title: Obsidian plugin via hand-rolled LSP bridge
status: "🔲"
model: opus
summary: >-
  Ship a desktop-only Obsidian plugin under
  `editors/obsidian/` that spawns `mdsmith lsp`, drives a
  hand-rolled JSON-RPC client, and renders diagnostics
  plus quick-fixes inside Obsidian's CodeMirror 6 editor.
depends-on: [121, 168]
---
# Obsidian plugin via hand-rolled LSP bridge

## Goal

Surface mdsmith inside Obsidian. A writer opening a
`.md` file sees inline squiggles. A "Fix" lightbulb
applies the matching quick-fix. Fix-on-save runs
`fixAll` on the active buffer. The plugin reuses
the LSP server from
[plan 121](121_vscode-integration.md) unchanged.

## Background

Plan 121 shipped `mdsmith lsp` and a VS Code client.
Plan 168 added the `obsidian` convention. Together
they cover the rules. The gap is the editor wiring.

Obsidian runs on Electron. Plugins are bundled to
one `main.js` plus `manifest.json`, loaded from
`<vault>/.obsidian/plugins/<id>/`. Desktop plugins
have Node access. So spawning `mdsmith lsp` via
`child_process` works. No `vscode-languageclient`
analog exists in the Obsidian ecosystem. The plugin
hand-rolls JSON-RPC framing and dispatch.

[Mobile Obsidian][mob] runs no Node. It cannot
spawn binaries. The manifest sets
`isDesktopOnly: true`. Mobile is the scope of
[plan 215](215_obsidian-wasm-mobile.md).

Ship via GitHub Release only. The zip holds
`main.js`, `manifest.json`, and `styles.css`. No
PR to [obsidian-releases][cp].

[mob]: https://help.obsidian.md/mobile
[cp]: https://github.com/obsidianmd/obsidian-releases

## Design

### Layout

`editors/obsidian/` mirrors `editors/vscode/`:

```text
editors/obsidian/
  manifest.json
  package.json
  tsconfig.json
  build.ts
  src/
    main.ts
    lsp-client.ts
    diagnostics.ts
    actions.ts
    settings.ts
    binary.ts
    *.test.ts
  styles.css
  README.md
```

The build emits one CommonJS bundle to
`dist/main.js`. Obsidian requires CommonJS. Static
files are copied as-is. The release zip holds
those three files plus the staged binaries.

### Binary resolution

Reuse the VS Code bundle path. The shared
`@mdsmith/cli` shim maps host to target. `build.ts`
copies `npm/mdsmith/bin/mdsmith.js` and stages each
platform binary under `dist/cli/@mdsmith/<target>/`.
`binary.ts` loads the shim and calls
`resolveBinary(process.platform, process.arch, …)`.

The resolver falls back to a `mdsmith.path` setting,
then `$PATH`. The fallback notice points at the
[releases page][rel].

[rel]: https://github.com/jeduden/mdsmith/releases

### JSON-RPC client

`lsp-client.ts` owns the JSON-RPC surface. It
spawns the binary, frames messages with
`Content-Length` headers, and dispatches replies
by id. The framing is around 80 lines. No
third-party package is used.

The client exposes:

- `spawn(binary, args, cwd)` and `kill()`.
- `request(method, params): Promise<unknown>`.
- `notify(method, params): void`.
- `onNotification(method, handler)`.
- Lifecycle: `initialize` → `initialized` →
  requests → `shutdown` → `exit`.

Methods the plugin sends or receives:

| Direction | Method                            | Why                                      |
| --------- | --------------------------------- | ---------------------------------------- |
| → server  | `initialize`                      | Handshake                                |
| → server  | `textDocument/didOpen`            | Buffer opened                            |
| → server  | `textDocument/didChange`          | Debounced edit                           |
| → server  | `textDocument/didSave`            | Triggers fix                             |
| → server  | `textDocument/didClose`           | Buffer closed                            |
| → server  | `textDocument/codeAction`         | Quick fixes plus `source.fixAll.mdsmith` |
| ← server  | `textDocument/publishDiagnostics` | Squiggles                                |
| ← server  | `window/showMessage`              | `new Notice(...)`                        |

Hover, completion, rename, and navigation are
deferred. Obsidian exposes those through different
surfaces. They warrant their own plan.

### Diagnostics in CodeMirror 6

Obsidian's source and live-preview editors both
use [CodeMirror 6][cm6]. The plugin adds a CM6
`StateField<DecorationSet>`. It holds active
diagnostics per file. Decorations apply a
severity-themed underline. Classes live in
`styles.css`.

Hover uses `hoverTooltip` from `@codemirror/view`.
The tooltip shows `code + message`. The footer
holds a "Fix" link. The link runs the same
code-action flow as the lightbulb.

[cm6]: https://codemirror.net/

A "mdsmith Diagnostics" [`ItemView`][iv] lists
workspace-wide diagnostics as a sortable table.
Click jumps to the source location.

[iv]: https://docs.obsidian.md/Plugins/User+interface/Views

### Code actions and fix-on-save

Obsidian has no lightbulb. Three surfaces stand in:

1. The hover tooltip "Fix" link.
2. Per-line palette commands. Each active
   diagnostic on the cursor line registers a
   transient `mdsmith: Fix — {code}` command. The
   set clears on cursor move.
3. The `mdsmith: Fix file` command. It sends
   `textDocument/codeAction` filtered to kind
   `source.fixAll.mdsmith` and applies the
   returned `WorkspaceEdit` — mirroring VS Code.

`fixOnSave` (off by default) debounces
`vault.on('modify')` and triggers `Fix file` 200 ms
after the last save. The command path is shared
with the palette entry.

### Settings

A `PluginSettingTab` renders five controls:

| Setting       | Default    | Purpose                    |
| ------------- | ---------- | -------------------------- |
| `binaryPath`  | `""`       | Override resolver          |
| `configPath`  | `""`       | Pass `-c` to the server    |
| `runMode`     | `"onSave"` | `onType`/`onSave`/`off`    |
| `fixOnSave`   | `false`    | Run `fixAll` after save    |
| `traceServer` | `"off"`    | `off`/`messages`/`verbose` |

Settings round-trip via `loadData` and `saveData`.
Changing `binaryPath` or `configPath` restarts the
server. Changing `runMode` or `fixOnSave`
reconfigures listeners without a restart.

### Lifecycle

`onload` reads settings, resolves the binary,
spawns the server, registers the CM6 extension,
registers commands and the diagnostics view, and
attaches vault listeners. `onunload` sends
`shutdown` and `exit`, kills the child after 1 s,
disposes views, and removes listeners. A
`mdsmith: Restart server` command exists for the
same reason VS Code has one.

### Build, test, release

`bun run build.ts --production` writes `dist/`,
stages the platform binaries, and zips the
artifact. CI runs `bun test`, then the build,
then attaches the zip to the GitHub Release.
The release pipeline picks it up the same way it
picks up the `.vsix`.

### Docs

A new `docs/guides/editors/obsidian.md` covers
install, settings, and troubleshooting. Update the
[linter comparison][lc] to cite the new plugin in
its Obsidian row. Add a note to the
[conventions reference][conv] that
`convention: obsidian` pairs with this plugin.
Mention the artifact in [github-releases.md][gh].

[lc]: ../docs/background/markdown-linters.md
[conv]: ../docs/reference/conventions.md
[gh]: ../docs/development/release-channels/github-releases.md

## Tasks

1. Scaffold `editors/obsidian/`: `package.json`,
   `tsconfig.json`, `manifest.json` with
   `isDesktopOnly: true`, `build.ts`, a stub
   `src/main.ts` extending `Plugin`, and a
   `README.md` that passes default rules.
2. Implement `lsp-client.ts`. Cover framing,
   request correlation, notification fan-out, and
   the `initialize`/`shutdown` cycle. Unit-test
   against an in-process `Duplex`.
3. Implement `binary.ts`. Load the `@mdsmith/cli`
   shim. Fall back to setting, then `$PATH`. Match
   the test surface of the VS Code module.
4. Implement `diagnostics.ts`. Add the CM6
   `StateField`, the effect type, and a
   `hoverTooltip` provider rendering code, message,
   and a Fix link.
5. Implement `actions.ts`. Add per-line palette
   commands from active diagnostics, the
   `Fix file` command via `executeCommand`, and the
   debounced `vault.on('modify')` handler.
6. Implement `settings.ts`. Wire the five controls,
   the `loadData`/`saveData` round-trip, and the
   restart-on-change for `binaryPath` and
   `configPath`.
7. Wire `main.ts`. `onload` spawns the server,
   registers the CM6 extension, the diagnostics
   view, the commands, and the settings tab.
   `onunload` cleans up.
8. Add `styles.css` for severity underlines and
   tooltip styling.
9. Add a `.github/workflows/` step that builds the
   plugin and uploads the zip as a release
   artifact, mirroring the existing `vscode` job.
10. Write `docs/guides/editors/obsidian.md`.
    Update the conventions reference, the
    linter-comparison page, and the GitHub
    Releases page.
11. Run `mdsmith fix .` and confirm `mdsmith check
    .` passes against the updated `PLAN.md`.

## Acceptance Criteria

- [ ] `editors/obsidian/` builds with `bun run
      build.ts --production`. The output is
      `dist/main.js`, `manifest.json`, and
      `styles.css`.
- [ ] `bun test` passes. The suite covers framing,
      binary resolution, diagnostics decoration,
      and settings round-trip.
- [ ] Loading the plugin in a vault that holds an
      `MDS001` violation shows a wavy underline
      within 500 ms of opening the file. Manual
      smoke step.
- [ ] The hover tooltip shows the rule code and
      message. The "Fix" link applies the
      quick-fix.
- [ ] `mdsmith: Fix file` produces the same buffer
      as `mdsmith fix` on the same input.
- [ ] Toggling `fixOnSave: true` runs `Fix file`
      after each save without a plugin restart.
- [ ] Editing `.mdsmith.yml` re-lints open files
      without a restart. The
      `didChangeWatchedFiles` event is forwarded.
- [ ] `manifest.json` has `isDesktopOnly: true`.
      Mobile Obsidian treats the plugin as absent,
      not as a crash.
- [ ] CI attaches `mdsmith-obsidian-<version>.zip`
      to the release artifacts.
- [ ] `docs/guides/editors/obsidian.md` exists.
      The linter-comparison page cites the new
      plugin in its Obsidian row.
- [ ] `mdsmith check .` passes against the
      updated `PLAN.md`.

## Non-Goals

- LSP hover, completion, rename, and symbol
  navigation. Each goes in its own follow-up.
- Mobile support. That is plan 215.
- Submission to the Obsidian Community Plugins
  catalog. The chosen channel is GitHub Releases.
- Live-preview rendering changes.
- New rule bindings beyond what the `obsidian`
  convention activates.

## See also

- [Plan 121: VS Code via LSP](121_vscode-integration.md)
- [Plan 215: WASM for mobile](215_obsidian-wasm-mobile.md)
- [Linter comparison][lc]
