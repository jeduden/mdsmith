# mdsmith for VS Code

Inline mdsmith diagnostics and quick fixes for Markdown.

The extension spawns `mdsmith lsp` over stdio. It surfaces
lint diagnostics as inline squiggles. Each fixable rule
contributes a quick fix. A `source.fixAll.mdsmith` action
runs `mdsmith fix` on the whole buffer.

## Prerequisites

- VS Code 1.85 or later.
- **No separate `mdsmith` install** — the `.vsix` bundles a binary
  for every supported platform (Linux, macOS, and Windows on x64 and
  arm64) and selects yours at startup. It does this by re-using the
  `@mdsmith/cli` npm package's own resolver, so the extension and
  `npx @mdsmith/cli` always agree on which binary runs.

  Override only if you want a specific build: set `mdsmith.path` to an
  absolute path (e.g. from
  `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`,
  `npm install -g @mdsmith/cli`, or the
  [releases page](https://github.com/jeduden/mdsmith/releases)). The
  extension falls back to resolving `mdsmith` on `PATH` only if your
  platform was not bundled.

## Install

```bash
code --install-extension mdsmith-<version>.vsix
```

## Settings

| Setting                | Default     | Purpose                                                                                                                                                                        |
|------------------------|-------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `mdsmith.path`         | `"mdsmith"` | Binary path; default runs the bundled per-platform binary. Set an absolute path to override; falls back to PATH only if your platform was not bundled (e.g. `/go/bin/mdsmith`) |
| `mdsmith.config`       | `""`        | Override `-c` config path                                                                                                                                                      |
| `mdsmith.run`          | `"onSave"`  | When to lint: `onType`, `onSave`, or `off`                                                                                                                                     |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save                                                                                                                                          |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity                                                                                                                                                            |

See the
[full guide](https://github.com/jeduden/mdsmith/blob/main/docs/guides/editors/vscode.md)
for prerequisites, code actions, troubleshooting, and the
performance benchmark.

## Building from source

The extension uses [Bun](https://bun.sh) for both bundling
and tests:

```bash
bun install
bun test
bun run build.ts            # one-shot bundle to dist/
bun run build.ts --watch    # rebuild on change
```

`bunx --bun @vscode/vsce package` produces the `.vsix`.

## License

MIT.
