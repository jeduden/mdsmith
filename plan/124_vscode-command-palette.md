---
id: 124
title: Command palette and status bar for init, query, merge-driver, version
status: "🔲"
model: sonnet
summary: >-
  Wire the remaining mdsmith subcommands — `init`,
  `query`, `merge-driver install`, and `version` —
  as VS Code command-palette entries plus a
  status-bar item, so every CLI surface has an
  editor entry point.
---
# Command palette and status bar for init, query, merge-driver, version

## Goal

Add a VS Code command-palette entry for each
mdsmith subcommand that does not already have an
editor surface from plans 121 to 123, plus a
status-bar item that shows the active mdsmith
version and the resolved `.mdsmith.yml` path. After
this plan every CLI subcommand has a path from the
editor.

## Background

After the first three plans:

| Subcommand     | Editor surface             | Source plan |
|----------------|----------------------------|-------------|
| `check`        | diagnostics                | 121         |
| `fix`          | code actions               | 121         |
| `help`         | hover                      | 122         |
| `kinds`        | hover, CodeLens, tree view | 122 / 123   |
| `archetypes`   | tree view                  | 123         |
| `metrics`      | tree view                  | 123         |
| `query`        | not yet exposed            | this plan   |
| `init`         | not yet exposed            | this plan   |
| `merge-driver` | not yet exposed            | this plan   |
| `version`      | not yet exposed            | this plan   |

The remaining four subcommands share a shape: they
are one-shot actions, not document-state
projections. The right wire for them is the
command palette, with a status-bar item carrying
the always-visible version / config path.

## Design

### Command-palette entries

Each command is registered in
`editors/vscode/package.json` under
`contributes.commands` and bound to a handler in
`editors/vscode/src/commands/`.

| Command ID                    | Title                             | Handler                                                 |
|-------------------------------|-----------------------------------|---------------------------------------------------------|
| `mdsmith.init`                | mdsmith: Initialize config        | Run `mdsmith init` in the workspace root                |
| `mdsmith.query`               | mdsmith: Run query                | Prompt for CUE expression; show matches in a quick-pick |
| `mdsmith.mergeDriver.install` | mdsmith: Install Git merge driver | Run `mdsmith merge-driver install` after confirmation   |
| `mdsmith.version`             | mdsmith: Show version             | Print version + binary path to the output channel       |

### `mdsmith.init`

Runs `mdsmith init` against the first workspace
folder. If `.mdsmith.yml` already exists, the
extension shows a confirmation dialog rather than
overwriting silently. On success, opens the
generated file and offers a notification with a
"Reload mdsmith" button that re-fetches every
view (plan 123) and re-runs diagnostics on every
open document.

### `mdsmith.query`

Prompts the user for a CUE expression with the
input box. The resolved files run through `mdsmith
query <expr> <workspace>`. Matches appear in a
quick-pick; selecting an entry opens the file. A
"Save as task" button appends the query to a
`.vscode/tasks.json` task labelled
`mdsmith query: <expr>`.

A second palette entry,
`mdsmith: Run query (verbose)`, exposes the
`-v` flag. It streams skipped-file diagnostics to
the output channel.

### `mdsmith.mergeDriver.install`

Wraps the existing `mdsmith merge-driver install`
subcommand. The handler always runs through a
confirmation dialog because the command writes to
`.gitattributes` and `.git/config`. After install,
the extension shows a notification linking to
[the workflow doc](../docs/development/pr-fixup-workflow.md).

### `mdsmith.version`

Runs `mdsmith version` and prints to the mdsmith
output channel. Also drives the status-bar item.

### Status-bar item

A single status-bar item in the right group shows
`mdsmith vX.Y.Z`. Tooltip lists:

- The resolved binary path (`mdsmith.path`
  setting).
- The version reported by `mdsmith version`.
- The active `.mdsmith.yml` path resolved by the
  LSP server (sent over a custom
  `mdsmith/configPath` notification at server
  startup).

Clicking the item runs `mdsmith.version`. The item
hides on workspaces without a Markdown file.

### LSP server changes

A small server-side addition: the server emits a
`mdsmith/configPath` notification after
`initialize` with the absolute path of the loaded
config (or an empty string if none was found). This
lets the status bar update without the extension
re-walking the tree.

## Tasks

1. Add a `mdsmith/configPath` LSP notification on
   the server. Send it once after `initialize` and
   again whenever the watched `.mdsmith.yml`
   changes. Cover with an integration test in
   [`cmd/mdsmith`](../cmd/mdsmith).
2. Add the four `mdsmith.*` commands to
   `editors/vscode/package.json` and write each
   handler in
   `editors/vscode/src/commands/`. Each handler
   spawns the binary, surfaces stderr in the
   output channel, and shows a notification on
   non-zero exit.
3. Add the status-bar item with the tooltip
   described above. Register the
   `mdsmith/configPath` notification listener and
   refresh the tooltip on every notification.
4. Add VS Code extension tests for each handler:
   `init` with and without an existing config,
   `query` with a matching and non-matching
   expression, `mergeDriver.install` with the
   confirmation dialog mocked, `version` against a
   stubbed binary.
5. Update the VS Code guide
   `docs/guides/editors/vscode.md` with one
   section per command, including the confirmation
   dialogs and the tasks-json hand-off for
   `query`.
6. Update the Commands table at the top of the
   guide so every CLI subcommand lists its editor
   surface, mirroring the table in this plan's
   Background section.

## Acceptance Criteria

- [ ] The command palette lists all four
      `mdsmith.*` commands in a workspace with a
      Markdown file open.
- [ ] `mdsmith: Initialize config` writes
      `.mdsmith.yml` in the workspace root and
      prompts before overwriting an existing file.
- [ ] `mdsmith: Run query` accepts a CUE
      expression, shows matching files in a
      quick-pick, and opens the chosen file.
- [ ] `mdsmith: Install Git merge driver` runs
      only after the confirmation dialog and
      surfaces stderr on failure.
- [ ] The status-bar item shows the version, hides
      on non-Markdown workspaces, and updates when
      `.mdsmith.yml` changes.
- [ ] Every CLI subcommand listed in
      [docs/reference/cli.md](../docs/reference/cli.md)
      has an entry in the editor surfaces table in
      the VS Code guide.
- [ ] All tests pass: `go test ./...` and
      `npm test` inside `editors/vscode/`.
- [ ] `go tool golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.

## Open Questions

- **Workspace trust.** `mdsmith.mergeDriver.install`
  modifies Git config. VS Code's restricted-mode
  workspaces should disable this command; pick the
  right `untrustedWorkspaces` capability flag during
  implementation.
- **Multi-root workspaces.** `mdsmith.init` targets
  the first workspace folder. Decide whether to
  prompt for a folder when more than one is open,
  or always require an explicit folder argument.

## ...

<?allow-empty-section?>
