# mdsmith for Obsidian

A desktop-only Obsidian plugin that runs the bundled mdsmith binary
as an LSP server. Diagnostics appear inline as squiggles in the
CodeMirror 6 editor, hover tooltips show the rule code and message,
and a Fix command applies the matching quick fix. A whole-buffer fix
runs on demand or after save.

## Prerequisites

- Desktop Obsidian 1.5 or later. Mobile Obsidian has no Node and
  cannot spawn the LSP binary — see plan 215 for the WASM successor.
- No separate `mdsmith` install. The release zip bundles a binary
  for every supported platform (Linux, macOS, Windows on x64 and
  arm64) and selects yours at startup. Override with the
  `binaryPath` setting if you want a specific build.

## Install

Download `mdsmith-obsidian-<version>.zip` from the
[releases page](https://github.com/jeduden/mdsmith/releases) and
extract it under
`<vault>/.obsidian/plugins/mdsmith/`. Enable the plugin from
Settings > Community plugins.

## Settings

| Setting       | Default    | Purpose                                    |
| ------------- | ---------- | ------------------------------------------ |
| `binaryPath`  | `""`       | Override the bundled binary                |
| `configPath`  | `""`       | Pass `-c <path>` to the server             |
| `runMode`     | `"onSave"` | `onType`, `onSave`, or `off`               |
| `fixOnSave`   | `false`    | Run Fix file after every save              |
| `traceServer` | `"off"`    | LSP trace: `off`, `messages`, or `verbose` |

## Building from source

The plugin uses [Bun](https://bun.sh) for tests and bundling.

```bash
bun install
bun test
bun run build.ts            # one-shot bundle to dist/
bun run build.ts --watch    # rebuild on change
bun run build.ts --production
```

## License

MIT.
