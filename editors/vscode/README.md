# mdsmith for VS Code

Inline mdsmith diagnostics and quick fixes for Markdown.

The extension spawns `mdsmith lsp` over stdio. It surfaces
lint diagnostics as inline squiggles. Each fixable rule
contributes a quick fix. A `source.fixAll.mdsmith` action
runs `mdsmith fix` on the whole buffer.

## Prerequisites

- VS Code 1.85 or later.
- **The `mdsmith` binary** — the extension includes a bundled binary
  for the host platform (typically Linux from CI builds). If you're on
  the same platform as the build host, no separate install is required.

  For other platforms or if the bundled binary is unavailable, install
  `mdsmith` manually:
  - `npm install -g @mdsmith/cli`
  - `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`
  - Download from the
    [releases page](https://github.com/jeduden/mdsmith/releases)
  - Then optionally configure `mdsmith.path` to point to the binary.

## Install

```bash
code --install-extension mdsmith-<version>.vsix
```

## Settings

| Setting                | Default     | Purpose                                                                                                                                                                 |
|------------------------|-------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `mdsmith.path`         | `"mdsmith"` | Binary path; defaults to bundled binary in dist/bin/. Falls back to PATH resolution if bundled binary unavailable. Set absolute path if needed (e.g. `/go/bin/mdsmith`) |
| `mdsmith.config`       | `""`        | Override `-c` config path                                                                                                                                               |
| `mdsmith.run`          | `"onSave"`  | When to lint: `onType`, `onSave`, or `off`                                                                                                                              |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save                                                                                                                                   |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity                                                                                                                                                     |

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
