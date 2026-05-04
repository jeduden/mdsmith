---
title: VS Code Integration
summary: >-
  Install the mdsmith VS Code extension, configure how
  it spawns `mdsmith lsp`, and read diagnostics inline
  as you edit Markdown files.
---
# VS Code Integration

The mdsmith VS Code extension surfaces lint
diagnostics inline as squiggles in the editor and
exposes `mdsmith fix` as code actions. The extension
itself is a thin client. It spawns the `mdsmith lsp`
subcommand over stdio and sends Language Server
Protocol messages; the lint pipeline runs in the Go
process, not the Node runtime.

The same server speaks vanilla LSP, so any LSP-aware
editor (Neovim, Helix, JetBrains via the LSP plugin)
gets the same behavior by pointing at `mdsmith lsp`.

## Prerequisites

- `mdsmith` binary on `$PATH`, or a path you supply
  to `mdsmith.path` in VS Code settings. Build with
  `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`
  or download from the
  [GitHub releases page](https://github.com/jeduden/mdsmith/releases).
- VS Code 1.85 or later.
- A `.mdsmith.yml` reachable from the workspace root
  by walking up to the nearest `.git` directory. The
  server matches the CLI's discovery (the same
  `config.Discover` walk) but starts from the workspace
  root supplied at `initialize`, not from each open
  document. Every open buffer in the workspace shares
  the resolved config.

## Install

The extension is shipped as a `.vsix` artifact next
to the Go binaries on each release. Install it with:

```bash
code --install-extension mdsmith-<version>.vsix
```

Marketplace publication is gated on release planning;
until then the `.vsix` is the supported install path.

## Settings

The extension contributes the following settings.
Project-level overrides go in `.vscode/settings.json`;
global preferences go in your user settings.

| Setting                | Default     | Purpose                                                                     |
|------------------------|-------------|-----------------------------------------------------------------------------|
| `mdsmith.path`         | `"mdsmith"` | Binary path; resolved against the extension-host PATH (see Troubleshooting) |
| `mdsmith.config`       | `""`        | Override `-c` config path (absolute or workspace)                           |
| `mdsmith.run`          | `"onSave"`  | When to lint: `onType`, `onSave`, or `off`                                  |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save                                       |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity: `off`, `messages`, `verbose`                           |

`mdsmith.path` is read by the extension to spawn the
server. The remaining settings are pulled by the
server via `workspace/configuration`. Changing any of
them takes effect on the next document event without
reloading the window.

The default `mdsmith.run` is `onSave`. Live linting
on every keystroke is opt-in because the latency
budget is tighter (see [Performance](#performance)).

## Code actions

The server advertises two action kinds.

**Quick fix per diagnostic.** Each diagnostic from a
fixable rule produces a `WorkspaceEdit`. Trigger it
with the lightbulb on a squiggle or
`editor.action.quickFix`. The action title reads
"Fix all `<rule>` with mdsmith" — the edit replaces
the entire document with the output of running the
single rule's fix, so it covers every occurrence of
that rule, not only the diagnostic the user clicked
on. Rules use that scope because mdsmith's fix
pipeline is whole-file: a rule emits the corrected
document, not a per-range diff.

Within one `codeAction` request the server runs each
rule's fix exactly once, regardless of how many
diagnostics from that rule are present. All
quick-fix actions for the same rule reference the
same `WorkspaceEdit`. Rules whose fix touches
multiple non-contiguous ranges (catalog, toc,
include) are excluded from quick fixes — they only
surface as whole-file actions.

**Whole-file fix.** The action kind
`source.fixAll.mdsmith` runs `mdsmith fix` on the
buffer and returns the diff as a `WorkspaceEdit`.
This matches the contract VS Code's "Fix all"
command expects. Bind it to save by setting:

```jsonc
{
  "editor.codeActionsOnSave": {
    "source.fixAll.mdsmith": "explicit"
  }
}
```

Or set `mdsmith.fixOnSave` to `true`, which wires the
same behavior without touching `editor.codeActionsOnSave`.

## Configuration discovery

The server starts at the workspace root supplied at
`initialize` (`rootUri` or the first workspace
folder) and walks upward until it finds a
`.mdsmith.yml` or hits a `.git` boundary — the same
walk `mdsmith check` uses from the CWD. Discovery is
workspace-wide, not per-document: every open buffer
shares the resolved config.

Setting `mdsmith.config` to a non-empty path skips
the walk entirely; relative paths resolve against
the workspace root.

Edits to `.mdsmith.yml` re-lint every open document
immediately. The server subscribes to
`**/.mdsmith.yml` via
`workspace/didChangeWatchedFiles`, invalidates its
cached config on a change event, and republishes
diagnostics for every open buffer in the same
handler — no extra edit or focus event is required.
The watcher's glob is rooted at the workspace, so
edits to a `.mdsmith.yml` outside the workspace (for
example a shared file pointed at via
`mdsmith.config`) do not trigger a re-lint; reload
the window or save any open Markdown buffer to force
one.

## Diagnostic mapping

mdsmith JSON diagnostics map to LSP `Diagnostic`
fields as follows:

| mdsmith field    | LSP field                                                               |
|------------------|-------------------------------------------------------------------------|
| `rule` + `name`  | `code` (e.g. `MDS001`); `source` is `"mdsmith"`                         |
| `message`        | `message`                                                               |
| `severity`       | `severity` (error → 1, warning → 2)                                     |
| `line`, `column` | `range.start`; end is the line's UTF-16 length (squiggle → end-of-line) |
| rule name        | `data.rule` (echoed back on `codeAction`)                               |

The same JSON schema documented in
[Output and JSON](../../reference/cli.md#output)
drives both the CLI and the LSP server. If you see a
diagnostic shape over LSP that does not match the
CLI, file an issue.

## Commands

The extension contributes two commands available in the
Command Palette under the `mdsmith:` category:

| Command                            | Action                                                                                                                                                                 |
|------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `mdsmith: Restart Language Server` | Stop the LSP client and spawn a fresh one. Use after editing `mdsmith.path`, rebuilding the binary, or recovering from a startup failure without reloading the window. |
| `mdsmith: Show Output Channel`     | Reveal the "mdsmith" Output channel where the client logs RPC traffic and the server's stderr. Quickest way to read a startup error.                                   |

## Troubleshooting

**No diagnostics appear.** Confirm the binary
resolves: open the integrated terminal and run
`mdsmith version`. If the command is not found, set
`mdsmith.path` to an absolute path. Check the LSP
trace by setting `mdsmith.trace.server` to
`messages`; the trace appears in the Output panel
under "mdsmith".

**`spawn mdsmith ENOENT`.** `which mdsmith` works in
the terminal but the extension still fails to spawn.
VS Code's extension host is a non-interactive Node
process that inherits the container/login-shell
PATH. It does NOT source `~/.bashrc` or `~/.zshrc`,
so `PATH` entries added there (a Go install often
puts the binary in `/go/bin` or `~/go/bin`) are
invisible to the extension. Either:

- Set `mdsmith.path` to the absolute path
  (`/go/bin/mdsmith`, `~/go/bin/mdsmith`, …); or
- Symlink the binary into a directory that is on the
  default PATH:
  `sudo ln -sf "$(which mdsmith)" /usr/local/bin/mdsmith`,
  then `Developer: Reload Window`.

**"Download mdsmith" error.** The extension cannot
find the binary. Either install it as above or set
`mdsmith.path` explicitly. The extension does not
bundle the binary because the Go executable is
platform-specific and a single `.vsix` should not
ship six binaries.

**"mdsmith server crashed too many times in a row."**
The extension's restart limiter (25 close events in
3 minutes) tripped, typically because the binary is
crashing on every request. Open the "mdsmith" Output
channel to read the server's stderr (panic stack
trace, "mdsmith: lsp:" line, etc.), fix the
underlying issue, then run `mdsmith: Restart
Language Server` from the Command Palette to bring
the LSP back online without reloading the window.

**Diagnostics lag behind edits.** Per-document lint
runs are debounced 200 ms after the last
`didChange`. If the editor still feels slow on large
files, switch `mdsmith.run` from `onType` to
`onSave`. Run the latency benchmark below to
characterize your environment before filing a bug.

**Quick fix does nothing.** A few rules
(catalog, toc, include) only expose whole-file
fixes. Use `source.fixAll.mdsmith` or run
`mdsmith fix <file>` from the terminal.

**Config edits outside the workspace do not take
effect.** The watcher's glob is rooted at the
workspace, so a `.mdsmith.yml` referenced via
`mdsmith.config` from elsewhere on disk does not
trigger a re-lint when edited. Reload the window or
save any open Markdown buffer to force one.

## Performance

Latency budget for the squiggle update path
(`didChange` → `publishDiagnostics`) is p95 under
150 ms on a 1 000-line file and under 500 ms on a
5 000-line file. The benchmark measuring this lives
under `internal/lsp/`. Run it locally with:

```bash
go test -run=^$ -bench=. ./internal/lsp/...
```

`go test ./...` does not invoke benchmarks by
default. CI runs the benchmark explicitly and fails
if the p95 exceeds the budgets above; missing the
budget blocks the default `mdsmith.run` from
flipping to `onType`.

The server itself is single-process, multi-document.
One client equals one server. Memory is bounded by
`GOMEMLIMIT` (512 MB), the same limit the CLI sets.

## See also

- [`mdsmith check`](../../reference/cli/check.md) —
  the CLI surface that the LSP server reuses
- [`mdsmith fix`](../../reference/cli/fix.md) — the
  fix pipeline behind both the per-diagnostic and
  whole-file code actions
- [Markdown linter comparison](../../background/markdown-linters.md)
  — how mdsmith editor support compares to peers
- [plan 121](../../../plan/121_vscode-integration.md)
  — design notes, task list, and acceptance
  criteria for this integration
