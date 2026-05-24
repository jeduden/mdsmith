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

- No separate `mdsmith` install. The `.vsix` bundles a
  binary for every supported platform (Linux, macOS,
  Windows on x64 and arm64) and selects yours at
  startup by re-using the `@mdsmith/cli` npm package's
  own resolver, so the editor and `npx @mdsmith/cli`
  always run the same binary. Override only to pin a
  specific build: set `mdsmith.path` to an absolute
  path (from
  `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`,
  `npm install -g @mdsmith/cli`, or the
  [GitHub releases page](https://github.com/jeduden/mdsmith/releases)).
- VS Code 1.85 or later.
- A `.mdsmith.yml` reachable from the workspace root
  by walking up to the nearest `.git` directory. The
  server matches the CLI's discovery (the same
  `config.Discover` walk) but starts from the workspace
  root supplied at `initialize`, not from each open
  document. Every open buffer in the workspace shares
  the resolved config.

## Install

Each release publishes the extension to the Visual
Studio Marketplace, to Open VSX, and as a `.vsix`
attached to the GitHub release. Pick one:

```bash
# stock VS Code, GitHub Codespaces, GitHub.dev (Marketplace)
code --install-extension jeduden.mdsmith
# Cursor, VSCodium, Theia, Gitpod (Open VSX)
codium --install-extension jeduden.mdsmith
# Air-gapped or version-pinned: download from the release page
code --install-extension mdsmith-<version>.vsix
```

The Marketplace, Open VSX, and GitHub-release `.vsix`
have identical SHA-256 sums; the same artifact is
uploaded to all three. See
[Installation: VS Code extension](../install.md#vs-code-extension)
for the channel-by-channel breakdown.

## Settings

The extension contributes the following settings.
Project-level overrides go in `.vscode/settings.json`;
global preferences go in your user settings.

| Setting                | Default     | Purpose                                                                                                 |
| ---------------------- | ----------- | ------------------------------------------------------------------------------------------------------- |
| `mdsmith.path`         | `"mdsmith"` | Default runs the bundled per-platform binary; an absolute path overrides it (see Troubleshooting)       |
| `mdsmith.config`       | `""`        | Override `-c` config path (absolute or workspace)                                                       |
| `mdsmith.run`          | `"onSave"`  | When to lint: `onType`, `onSave`, or `off`                                                              |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save                                                                   |
| `mdsmith.previewFix`   | `false`     | Open Refactor Preview before applying any fix (see [Preview before applying](#preview-before-applying)) |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity: `off`, `messages`, `verbose`                                                       |

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
same `WorkspaceEdit`. Generated-section rules
(catalog, toc, include) regenerate the section in
their fix — the quick-fix action surfaces normally,
labelled "Fix all `<rule>` with mdsmith" so the
whole-file scope is explicit.

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

## Preview before applying

Set `mdsmith.previewFix` to `true` to route every fix
through VS Code's Refactor Preview pane. With the
setting on, each code action — whether triggered from
the lightbulb or from `editor.codeActionsOnSave` —
carries a `ChangeAnnotation` flagged
`needsConfirmation: true`. VS Code opens the diff
pane before writing any change. You accept or reject
per action.

The preview requires a client that advertises both
`documentChanges` and `changeAnnotationSupport` in
the LSP `initialize` handshake (VS Code 1.85 or
later). On older clients, or on editors that do not
advertise both capabilities, the server falls back to
the immediate-apply form and logs one warning to the
Output channel naming the missing capability.

The setting interacts with `mdsmith.fixOnSave` as
follows:

| `previewFix` | `fixOnSave` | Behavior                                  |
| ------------ | ----------- | ----------------------------------------- |
| `false`      | `false`     | Click lightbulb → fix applied immediately |
| `false`      | `true`      | Save → fix applied immediately            |
| `true`       | `false`     | Click lightbulb → Refactor Preview opens  |
| `true`       | `true`      | Save → Refactor Preview opens             |

Leave `mdsmith.previewFix` off (the default) for
day-to-day use. Turn it on temporarily when reviewing
a batch of fixes, or in a project where you want
explicit confirmation before any automated edit lands.

## Outline and Go to Definition

The server publishes a hierarchical outline of each
open Markdown file. It also resolves cross-document
jumps. VS Code surfaces these as the Outline pane,
"Go to Definition" (F12), "Find All References"
(Shift-F12), the `Ctrl-T`/`Cmd-T` symbol picker, and
the call-hierarchy view.

The relevant LSP methods are:

- `textDocument/documentSymbol`
- `textDocument/definition`
- `textDocument/implementation`
- `textDocument/references`
- `workspace/symbol`
- `textDocument/prepareCallHierarchy`

Headings nest by level. Front-matter keys hang off a
synthetic "front matter" entry. Directives
(`<?include?>`, `<?catalog?>`, `<?build?>`) attach to
their enclosing heading or to the file root. See the
[`mdsmith lsp` reference](../../reference/cli/lsp.md#symbol-navigation)
for the symbol-kind table and the cursor → target
matrix.

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
| ---------------- | ----------------------------------------------------------------------- |
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

The extension contributes commands available in the
Command Palette under the `mdsmith:` category.

### Server commands

| Command                            | Action                                                                                                                                                                 |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `mdsmith: Restart Language Server` | Stop the LSP client and spawn a fresh one. Use after editing `mdsmith.path`, rebuilding the binary, or recovering from a startup failure without reloading the window. |
| `mdsmith: Show Output Channel`     | Reveal the "mdsmith" Output channel where the client logs RPC traffic and the server's stderr. Quickest way to read a startup error.                                   |

### Palette commands

| Command                              | Requires trust | Action                                                                         |
| ------------------------------------ | :------------: | ------------------------------------------------------------------------------ |
| `mdsmith: Initialize Config`         | yes            | Run `mdsmith init` in the workspace root to create `.mdsmith.yml`.             |
| `mdsmith: Install Git Merge Driver`  | yes            | Run `mdsmith merge-driver install` after a confirmation dialog.                |
| `mdsmith: Fix All Markdown`          | yes            | Run `mdsmith fix .` against the workspace; show a fixed-of-total summary.      |
| `mdsmith: Explain Rule on This File` | —              | Pick a rule; open `mdsmith kinds why --json -- <file> <rule>` in a side panel. |
| `mdsmith: Show Resolved Config`      | —              | Open `mdsmith kinds resolve --json -- <file>` in a side panel.                 |

`mdsmith: Initialize Config`, `mdsmith: Fix All Markdown`, and
`mdsmith: Install Git Merge Driver` are hidden from the palette in
untrusted workspaces. `mdsmith: Fix All Markdown` and
`mdsmith: Install Git Merge Driver` also show a confirmation dialog
before modifying files.

`mdsmith: Explain Rule on This File` and
`mdsmith: Show Resolved Config` are visible only when a Markdown
file is active. Both open a read-only Markdown panel populated
from JSON output; closing the panel discards the buffer.

### CLI-subcommand coverage

| mdsmith subcommand | Editor entry point                        |
| ------------------ | ----------------------------------------- |
| `check`            | Inline diagnostics (squiggles)            |
| `fix`              | Code actions; `mdsmith: Fix All Markdown` |
| `lsp`              | Extension spawns automatically            |
| `init`             | `mdsmith: Initialize Config`              |
| `merge-driver`     | `mdsmith: Install Git Merge Driver`       |
| `kinds resolve`    | `mdsmith: Show Resolved Config`           |
| `kinds why`        | `mdsmith: Explain Rule on This File`      |
| `help` (rules)     | Hover (plan 133)                          |
| `metrics`          | CLI only                                  |
| `query`            | CLI only                                  |
| `version`          | CLI only                                  |

## Troubleshooting

**No diagnostics appear.** Confirm the binary
resolves: open the integrated terminal and run
`mdsmith version`. If the command is not found, set
`mdsmith.path` to an absolute path. Check the LSP
trace by setting `mdsmith.trace.server` to
`messages`; the trace appears in the Output panel
under "mdsmith".

**`spawn mdsmith ENOENT`.** Only reachable when the
extension falls back to `PATH` — i.e. you set
`mdsmith.path` to a bare name, or your platform was
not bundled. `which mdsmith` works in the terminal
but the extension still fails to spawn: VS Code's
extension host is a non-interactive Node process that
inherits the container/login-shell PATH. It does NOT
source `~/.bashrc` or `~/.zshrc`, so `PATH` entries
added there (a Go install often puts the binary in
`/go/bin` or `~/go/bin`) are invisible to the
extension. Either:

- Clear `mdsmith.path` (empty or `"mdsmith"`) so the
  extension uses the binary bundled in the `.vsix`; or
- Set `mdsmith.path` to the absolute path
  (`/go/bin/mdsmith`, `~/go/bin/mdsmith`, …); or
- Symlink the binary into a directory that is on the
  default PATH:
  `sudo ln -sf "$(which mdsmith)" /usr/local/bin/mdsmith`,
  then `Developer: Reload Window`.

An empty `mdsmith.path` is safe: it resolves to the
bundled binary rather than crashing the language
client with the old `Unsupported server configuration
{ command: "" }` error.

**"Download mdsmith" error.** The `.vsix` ships a
binary for every supported platform, so this only
appears if your OS/arch is unsupported, the bundled
binary was stripped from the package, or a custom
`mdsmith.path` is wrong. Fix `mdsmith.path`, or
install `mdsmith` via `npm install -g @mdsmith/cli`,
`go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`,
or the
[releases page](https://github.com/jeduden/mdsmith/releases).

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

**Quick fix does nothing visible.** Confirm the
LSP is running (status bar should not say "mdsmith
disabled"). Open the "mdsmith" Output channel and
trigger a re-lint by editing the buffer; if no
JSON-RPC traffic appears, the server is not running.
For generated-section rules (catalog, toc, include),
the action regenerates the entire block — the
"diff" the editor applies is whole-file.

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
