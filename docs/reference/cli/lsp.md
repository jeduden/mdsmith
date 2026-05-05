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

| mdsmith          | LSP                                                                     |
|------------------|-------------------------------------------------------------------------|
| `rule` + `name`  | `code` (e.g. `MDS001`); `source = mdsmith`                              |
| `severity`       | `severity` (error → 1, warning → 2)                                     |
| `line`, `column` | `range.start`; end is the line's UTF-16 length (squiggle → end-of-line) |
| `message`        | `message`                                                               |
| rule name        | `data.rule` (echoed back on codeAction)                                 |

## Code actions

- **`quickfix`** — one per fixable diagnostic. Each
  edit replaces the whole document with the output of
  running the single rule, so it covers every
  occurrence of that rule (the action title reads
  "Fix all `<rule>` with mdsmith"). Within one
  request all quick-fix actions for the same rule
  share one `WorkspaceEdit`; the fix is run once
  regardless of how many diagnostics carry that
  rule. Generated-section rules (catalog, toc,
  include) regenerate the section in their fix; the
  action surfaces normally and the title
  ("Fix all `<rule>` with mdsmith") is explicit
  about the whole-file scope.
- **`source.fixAll.mdsmith`** — runs `mdsmith fix` on the
  current buffer; produces the same bytes the on-disk fixer
  would write.

## Configuration discovery

The server uses workspace-wide discovery. It starts
at the workspace root supplied at `initialize`
(`rootUri` or the first `workspaceFolders` entry)
and walks upward until it finds a `.mdsmith.yml` or
hits a `.git` boundary — the same walk `mdsmith
check` uses from its CWD. Every open buffer shares
the resolved config; the server does not re-discover
per file.

Clients override the walk with `mdsmith.config`,
which the server pulls via `workspace/configuration`.
Edits to `.mdsmith.yml` invalidate the cached
config. The server then re-lints every open document
immediately.

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
