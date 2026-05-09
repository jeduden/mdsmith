# mdsmith for VS Code

Inline mdsmith diagnostics and quick fixes for Markdown.

The extension spawns `mdsmith lsp` over stdio. It surfaces
lint diagnostics as inline squiggles. Each fixable rule
contributes a quick fix. A `source.fixAll.mdsmith` action
runs `mdsmith fix` on the whole buffer.

## Prerequisites

- **VS Code 1.85 or later.**
- **The `mdsmith` binary** — the extension bundles pre-built binaries
  for all platforms (Linux, macOS, Windows) from npm. The build step
  copies platform binaries from the `@mdsmith/*` npm packages into
  `dist/bin/`, so they ship in the .vsix and work on all platforms
  from a single install. No separate binary install is required in
  most cases.

  If the bundled binary is unavailable or you prefer a custom build,
  you can install `mdsmith` manually:
  - `npm install -g @mdsmith/cli`
  - `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`
  - Download from the
    [releases page](https://github.com/jeduden/mdsmith/releases)
  - Then configure `mdsmith.path` to point to the binary.

## Install

```bash
code --install-extension mdsmith-<version>.vsix
```

## Settings

| Setting                | Default     | Purpose                                                                                                                      |
|------------------------|-------------|------------------------------------------------------------------------------------------------------------------------------|
| `mdsmith.path`         | `"mdsmith"` | Binary path; defaults to bundled binary in dist/bin/. Falls back to PATH resolution if bundled binary unavailable. Set absolute path if needed (e.g. `/go/bin/mdsmith`) |
| `mdsmith.config`       | `""`        | Override `-c` config path                                                                                                    |
| `mdsmith.run`          | `"onSave"`  | When to lint: `onType`, `onSave`, or `off`                                                                                   |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save                                                                                        |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity                                                                                                          |

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
