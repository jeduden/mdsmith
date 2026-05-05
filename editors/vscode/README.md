# mdsmith for VS Code

Inline mdsmith diagnostics and quick fixes for Markdown.

The extension spawns `mdsmith lsp` over stdio. It surfaces
lint diagnostics as inline squiggles. Each fixable rule
contributes a quick fix. A `source.fixAll.mdsmith` action
runs `mdsmith fix` on the whole buffer.

## Prerequisites

- The `mdsmith` binary on `$PATH`, or a path you supply via
  the `mdsmith.path` setting. Install with
  `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`
  or download from the
  [releases page](https://github.com/jeduden/mdsmith/releases).
- VS Code 1.85 or later.

## Install

```bash
code --install-extension mdsmith-<version>.vsix
```

## Settings

| Setting                | Default     | Purpose                                                                                                                                                                                                  |
|------------------------|-------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `mdsmith.path`         | `"mdsmith"` | Binary path; resolved against the extension-host PATH (use an absolute path if `which mdsmith` works in your terminal but the extension reports `spawn ENOENT` — `~/.bashrc`/`~/.zshrc` are not sourced) |
| `mdsmith.config`       | `""`        | Override `-c` config path                                                                                                                                                                                |
| `mdsmith.run`          | `"onSave"`  | When to lint: `onType`, `onSave`, or `off`                                                                                                                                                               |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save                                                                                                                                                                    |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity                                                                                                                                                                                      |

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
