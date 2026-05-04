---
command: lsp
summary: Run a Language Server Protocol server on stdio for editor integrations.
---
# `mdsmith lsp`

Run an LSP server that speaks the Language Server Protocol over
stdio. The server reuses the same lint and fix pipelines as
`check` and `fix`, surfaces diagnostics, and exposes per-rule
quick fixes plus a whole-file `source.fixAll.mdsmith` action.

```text
mdsmith lsp
```

The subcommand takes no arguments. Designed to be spawned by an
LSP client (VS Code, Neovim, Helix, JetBrains LSP plugin), not
run interactively. It reads JSON-RPC frames on stdin and writes
responses and notifications on stdout.

## Capabilities advertised

| Capability                        | Behavior                                                      |
|-----------------------------------|---------------------------------------------------------------|
| `textDocumentSync = Full`         | Full-document sync; lint trigger gated by `mdsmith.run`       |
| `publishDiagnostics`              | One push after each lint                                      |
| `codeActionProvider`              | `quickfix` per fixable diagnostic, `source.fixAll.mdsmith`    |
| `workspace/didChangeWatchedFiles` | Immediate re-lint of open buffers when `.mdsmith.yml` changes |

`mdsmith.run` controls when the server actually re-lints:

- `onSave` (default): lint on `didOpen`, `didSave`, and config
  changes. `didChange` events update the buffer but do not trigger a
  lint pass.
- `onType`: lint on every `didChange` (debounced 200 ms) plus the
  same triggers as `onSave`.
- `off`: never lint automatically. Code actions still work when
  invoked explicitly.

## Diagnostic mapping

LSP `Diagnostic` fields map from the same JSON shape `check`
prints:

| mdsmith          | LSP                                        |
|------------------|--------------------------------------------|
| `rule` + `name`  | `code` (e.g. `MDS001`); `source = mdsmith` |
| `severity`       | `severity` (error → 1, warning → 2)        |
| `line`, `column` | `range.start`; end column derived per-rule |
| `message`        | `message`                                  |
| rule name        | `data.rule` (echoed back on codeAction)    |

## Code actions

- **`quickfix`** — one per fixable diagnostic. Rules whose fix
  touches multiple non-contiguous ranges (catalog, toc,
  include) are excluded so partial regenerations are not
  exposed.
- **`source.fixAll.mdsmith`** — runs `mdsmith fix` on the
  current buffer; produces the same bytes the on-disk fixer
  would write.

## Configuration discovery

The server uses workspace-wide discovery. It walks up
from the workspace root to find `.mdsmith.yml`. The root
comes from `initialize.rootUri` or the first
`workspaceFolders` entry. Every open buffer shares the
loaded config; the server does not re-discover per file.

Clients override the discovery with `mdsmith.config`. The
server pulls that path through `workspace/configuration`.
Edits to `.mdsmith.yml` invalidate the cached config. The
server then re-lints every open document immediately.

## Example

For client setup and troubleshooting see the
[VS Code guide](../../guides/editors/vscode.md). Other LSP
clients can spawn the binary directly:

```bash
mdsmith lsp
```

## Performance

The squiggle-update path is benchmarked under
`internal/lsp/`. Plan 121 sets a p95 budget of 150 ms on a
1 000-line buffer and 500 ms on a 5 000-line buffer. Run the
benchmark locally with:

```bash
go test -run=^$ -bench=. ./internal/lsp/...
```

## Exit codes

| Code | Meaning                    |
|------|----------------------------|
| 0    | Server exited cleanly      |
| 2    | Runtime or transport error |

## See also

- [`mdsmith check`](check.md) — the CLI surface that the
  server reuses
- [`mdsmith fix`](fix.md) — the fix pipeline behind both
  code actions
- [VS Code guide](../../guides/editors/vscode.md) — install,
  settings, troubleshooting
