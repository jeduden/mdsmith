# mdsmith for Obsidian

An [Obsidian](https://obsidian.md/) plugin that runs the
[mdsmith](https://mdsmith.dev) Markdown linter inside your vault.
Inline squiggles flag issues as you write, a hover tooltip offers a
one-click fix, and `mdsmith: Fix file` rewrites the active note. One
WebAssembly runtime powers desktop and mobile alike — no subprocess,
no native binary, no PATH lookup.

## How it works

The plugin loads the mdsmith engine compiled to WebAssembly (the same
build the [engine API](../../docs/background/concepts/engine-api.md)
exposes) and holds one `mdsmith.Session` for the vault. Editing a note
pushes the new bytes into the session; cross-file rules (broken links,
catalog drift) see the whole vault. Because the runtime is WASM, the
plugin works in Obsidian's sandboxed mobile WebView, where subprocess
spawning and native binary loading are both blocked. `manifest.json`
deliberately omits `isDesktopOnly`.

## Build

```sh
bun install
bun run build.ts --production
```

The build runs `cmd/mdsmith-wasm/build.sh` to make the `.wasm` file,
then bundles the plugin. Five files land in `dist/`: `main.js`,
`mdsmith.wasm`, `wasm_exec.js`, `manifest.json`, and `styles.css`.
Obsidian loads them from `<vault>/.obsidian/plugins/mdsmith/`.

## Test

```sh
bun test
```

## Settings

| Setting      | Default    | Purpose                          |
| ------------ | ---------- | -------------------------------- |
| `configPath` | `""`       | Override the `.mdsmith.yml` path |
| `runMode`    | `"onSave"` | `onType` / `onSave` / `off`      |
| `fixOnSave`  | `false`    | Run `Fix file` after each save   |

See the [user guide](../../docs/guides/editors/obsidian.md) for install
and troubleshooting.
