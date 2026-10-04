---
weight: 45
summary: >-
  Settings and troubleshooting for the mdsmith VS Code
  extension: the five `mdsmith.*` settings, fix-on-save
  wiring, and fixes for its known failure modes.
---
# mdsmith VS Code extension

The mdsmith VS Code extension is a thin LSP client over the
bundled `mdsmith` binary, which it runs with the `lsp`
subcommand. This page lists the extension's settings and the
fixes for its known failure modes. For what the extension
does and how to install it, see the
[VS Code guide](../guides/editors/vscode.md).

## Settings

Project overrides go in `.vscode/settings.json`; global
preferences go in user settings. A changed setting takes
effect on the next document event, with no window reload.

| Setting                | Default   | Purpose                                                                                   |
| ---------------------- | --------- | ----------------------------------------------------------------------------------------- |
| `mdsmith.run`          | `onType`  | When to lint: `onType` (default), `onSave`, or `off` (off stops automatic linting)        |
| `mdsmith.previewFix`   | `false`   | Show the diff (Refactor Preview) before fix-on-save writes; quick fixes apply immediately |
| `mdsmith.config`       | `""`      | Override the `.mdsmith.yml` path (absolute or workspace)                                  |
| `mdsmith.path`         | `mdsmith` | Pin a binary; the default runs the bundled per-platform one                               |
| `mdsmith.trace.server` | `off`     | LSP trace verbosity: `off`, `messages`, or `verbose`                                      |

`mdsmith.run` controls automatic linting. `onType` updates
diagnostics live as text changes. `onSave` defers them to
save. `off` stops automatic linting; quick fixes still run
on demand.

`mdsmith.config` overrides config discovery. Without it, the
server walks up from the workspace root to the nearest
`.mdsmith.yml` or `.git`, the same as `mdsmith check`.

## Fix on save

Fix-on-save is configured through VS Code's native
`editor.codeActionsOnSave`, not an mdsmith setting. The
[VS Code guide](../guides/editors/vscode.md) shows the
`source.fixAll.mdsmith` entry. The former `mdsmith.fixOnSave`
toggle is now a deprecated no-op.

Fix-on-save runs independently of `mdsmith.run`.
`mdsmith.previewFix` governs fix-on-save only. When it is
`true`, each save routes through VS Code's Refactor Preview
pane and shows the diff before the edit writes. Interactive
lightbulb quick fixes always apply immediately, since each is
the one fix just chosen.

## Troubleshooting

**No diagnostics appear.** The binary did not resolve. Run
`mdsmith version` in the integrated terminal. If the command
is not found, set `mdsmith.path` to an absolute path. Set
`mdsmith.trace.server` to `messages` and read the "mdsmith"
Output channel.

**`spawn mdsmith ENOENT`.** This is reachable only when
`mdsmith.path` is a bare name and the running platform was
not bundled. The extension host does not source `~/.bashrc`,
so a `go install` location such as `~/go/bin` is invisible to
it. Clear `mdsmith.path` to use the bundled binary, or set it
to an absolute path.

**Server crashed too many times.** The restart limiter
tripped because the binary crashes on every request. Open the
"mdsmith" Output channel for the stack trace, fix the cause,
then run **mdsmith: Restart Language Server**.

**Two mdsmith servers running.** A reload or update can leave
the old extension host alive next to the new one, each with
its own server. The newest server wins: it claims the
workspace, and the older one exits after sending an
`mdsmith/superseded` notice so the client does not restart
it.

The claim is scoped to the VS Code workspace by the
extension's per-workspace storage URI. A Claude Code
plugin or another editor on the same folder sends no scope,
so its server runs alongside VS Code's and neither stops the
other. Two VS Code windows on one folder share that URI, so
the newest of those still wins. An empty window with no folder
open sends no scope.

The storage URI follows the workspace, not the folder. Adding a
second folder to a one-folder window, or **Save Workspace As**,
gives the window a new URI. If the old extension host leaks
across that change, its server keeps the old scope and does not
step aside. Kill that host once, as for a pre-singleton orphan
below.

An orphan left by an older build that keyed on the folder
alone also steps aside when the new server starts. So does any
other server on such a build,
such as a plugin still on a cached older `mdsmith`. With
`mdsmith.path` set to a newer binary than an older extension
expects, no scope is sent and the orphan is not reaped until the
extension updates. A build from before the singleton leaves an
orphan that never steps aside: kill its extension host once —
not the `mdsmith` process, which the host respawns.

## See also

- [VS Code guide](../guides/editors/vscode.md) — what the
  extension does and how to install it
- [`mdsmith lsp`](cli/lsp.md) — the protocol reference:
  capabilities, diagnostic mapping, and symbol navigation
- [`mdsmith check`](cli/check.md) and [`mdsmith fix`](cli/fix.md)
  — the CLI surfaces the extension reuses
