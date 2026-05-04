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
- A `.mdsmith.yml` at the repo root or in a parent
  directory (the same file the CLI discovers).

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

| Setting                | Default     | Purpose                                           |
|------------------------|-------------|---------------------------------------------------|
| `mdsmith.path`         | `"mdsmith"` | Binary path; resolved against `$PATH`             |
| `mdsmith.config`       | `""`        | Override `-c` config path (absolute or workspace) |
| `mdsmith.run`          | `"onSave"`  | When to lint: `onType`, `onSave`, or `off`        |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save             |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity: `off`, `messages`, `verbose` |

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
fixable rule produces a `WorkspaceEdit` that applies
just that rule's fix to the affected range. Trigger
with the lightbulb on a squiggle, or
`editor.action.quickFix`. Rules whose fix touches
multiple non-contiguous ranges (catalog, toc,
include) are excluded from per-diagnostic actions to
avoid partial regenerations — they only surface as
whole-file actions.

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

The server walks up from the document URI to find a
`.mdsmith.yml`, the same way the CLI does. Setting
`mdsmith.config` to a non-empty path overrides this
walk; the path is resolved relative to the workspace
root.

Edits to `.mdsmith.yml` re-lint every open document
without restarting the editor. The server subscribes
to `**/.mdsmith.yml` via
`workspace/didChangeWatchedFiles`, invalidates its
cached config, and republishes diagnostics on the
next document change or focus.

## Diagnostic mapping

mdsmith JSON diagnostics map to LSP `Diagnostic`
fields as follows:

| mdsmith field    | LSP field                                       |
|------------------|-------------------------------------------------|
| `rule` + `name`  | `code` (e.g. `MDS001`); `source` is `"mdsmith"` |
| `message`        | `message`                                       |
| `severity`       | `severity` (error → 1, warning → 2)             |
| `line`, `column` | `range.start`; end column derived per-rule      |
| `explanation`    | `data` (preserved for code-action handlers)     |

The same JSON schema documented in
[Output and JSON](../../reference/cli.md#output)
drives both the CLI and the LSP server. If you see a
diagnostic shape over LSP that does not match the
CLI, file an issue.

## Troubleshooting

**No diagnostics appear.** Confirm the binary
resolves: open the integrated terminal and run
`mdsmith version`. If the command is not found, set
`mdsmith.path` to an absolute path. Check the LSP
trace by setting `mdsmith.trace.server` to
`messages`; the trace appears in the Output panel
under "mdsmith".

**"Download mdsmith" error.** The extension cannot
find the binary. Either install it as above or set
`mdsmith.path` explicitly. The extension does not
bundle the binary because the Go executable is
platform-specific and a single `.vsix` should not
ship six binaries.

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

**Config edits do not take effect.** The watcher
only fires for `.mdsmith.yml` paths inside the
workspace. If you edit a config outside the
workspace (e.g. via `mdsmith.config` pointing at a
shared file elsewhere), reload the window or save
any open Markdown buffer to force a re-lint.

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
