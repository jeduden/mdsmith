---
id: 122
title: VS Code palette commands
status: "✅"
model: sonnet
summary: >-
  A small set of VS Code command-palette entries —
  `init`, `merge-driver install`, `fix workspace`,
  `kinds why`, `kinds resolve` — that cover the
  remaining mdsmith subcommands without adding chrome
  to the editor. Hover help is split into plan 133.
---
# VS Code palette commands

## Goal

After plan 121 ships diagnostics and code actions
and plan 133 adds hover for rule docs, one question
stays unanswered from the editor: "how do I run a
mdsmith subcommand without leaving VS Code?". This
plan answers it with a short palette menu.

The plan deliberately ships no permanent UI chrome.
No CodeLens, no status-bar item, no activity-bar
panel. Reviewers across personas (app dev, OSS
maintainer, SRE) all rated permanent UI as the
first feature they would disable.

## Background

Plan 121 covers diagnostics and per-file fixes.
Plan 133 covers hover for `help rule` and
directive docs only — the rest of `mdsmith help`
(e.g. `help metrics`, `help kinds`,
`help concepts`) stays CLI-only since those topics
have no in-buffer anchor to hover over. Seven
subcommands stay outside the editor:
`help`, `kinds`, `metrics`,
`query`, `init`, `merge-driver`, `version`. A
reviewer audit grouped them:

- **Palette**: `init`, `merge-driver install`,
  fix-everything, `kinds why`, `kinds resolve`.
- **Hover (plan 133)**: `help rule <id>` and the
  directive-doc subset.
- **CLI only**: the rest of `help`, `metrics`,
  `query`, `version` — reviewers would
  uninstall a tree view or status-bar pill
  surfacing these.

## Design

### Palette commands

Each command is registered in
`editors/vscode/package.json` under
`contributes.commands` and bound to a handler in
`editors/vscode/src/commands/`.

| Command ID                    | Title                              | Action                                                          |
|-------------------------------|------------------------------------|-----------------------------------------------------------------|
| `mdsmith.init`                | mdsmith: Initialize config         | Run `mdsmith init` in the workspace root                        |
| `mdsmith.mergeDriver.install` | mdsmith: Install Git merge driver  | Run `mdsmith merge-driver install` after confirmation           |
| `mdsmith.fixWorkspace`        | mdsmith: Fix all Markdown          | Run `mdsmith fix .` against the workspace; show summary         |
| `mdsmith.kinds.why`           | mdsmith: Explain rule on this file | Pick a rule; open `mdsmith kinds why <file> <rule> --json` view |
| `mdsmith.kinds.resolve`       | mdsmith: Show resolved config      | Open `mdsmith kinds resolve <file> --json` virtual doc          |

Each handler spawns the binary, surfaces stderr in
a `mdsmith` output channel, and shows a
notification on non-zero exit. No status-bar
plumbing.

#### `mdsmith.fixWorkspace`

Fills the gap the SRE reviewer flagged. Per-buffer
`source.fixAll.mdsmith` cannot help when twenty
runbooks need the same trailing-space fix.

The handler runs `mdsmith fix .` from the
workspace root and parses the `stats:` line. The
CLI's `printRunStats` writes that line to stderr,
so the handler reads stderr or combined output.
The notification shows `Fixed 12 of 200 files` with
a "Show output" button.

A confirmation dialog gates the command, since it
touches files outside the active editor. Untrusted
workspaces skip the dialog and fail closed.

#### `mdsmith.kinds.why` and `mdsmith.kinds.resolve`

Both handlers open a read-only virtual document
(`mdsmith-kinds:` URI scheme) populated from the
JSON output of the corresponding subcommand,
rendered as Markdown. No state, no refresh logic.
Closing the tab discards the buffer.

For `kinds.why`, the handler shows a quick-pick of
diagnostic rule IDs on the active editor; the
selection drives `mdsmith kinds why <file> <rule>
--json`.

### Workspace trust

The extension declares `capabilities.untrustedWorkspaces`
in `editors/vscode/package.json`:

```jsonc
"capabilities": {
  "untrustedWorkspaces": {
    "supported": "limited",
    "description": "Diagnostics work in restricted mode. Commands that modify files outside the editor are disabled until the workspace is trusted.",
    "restrictedConfigurations": [
      "mdsmith.path",
      "mdsmith.config"
    ]
  }
}
```

- `"limited"` lets the language client load in an
  untrusted workspace; it never writes files.
- The two destructive palette commands hide
  behind `when: isWorkspaceTrusted` on their
  menu entries. Handlers re-check
  `workspace.isTrusted` before running.
- `restrictedConfigurations` blocks workspace
  overrides of `mdsmith.path` and `mdsmith.config`.
  An untrusted folder cannot redirect the
  extension to a malicious binary.
- The extension subscribes to
  `onDidGrantWorkspaceTrust`; gated commands
  appear without a reload after trust is granted.

### What this plan removes

| Earlier proposal                         | Decision        |
|------------------------------------------|-----------------|
| CodeLens at line 1 listing kinds         | Cut (chrome)    |
| Status-bar version + config-path pill    | Cut (chrome)    |
| `mdsmith` activity-bar container         | Cut (no demand) |
| `Kinds` / `Archetypes` / `Metrics` views | Cut (no demand) |
| `mdsmith.query` quick-pick prompt        | Cut (wrong UX)  |
| `mdsmith/configPath` LSP notification    | Cut (no caller) |

Each removal traces to unanimous reviewer
feedback.

## Tasks

1. [x] Register the five palette commands in
   `editors/vscode/package.json`. Add one handler
   per command in `editors/vscode/src/commands/`.
   Each spawns the binary, surfaces stderr, and
   shows a notification on non-zero exit.
2. [x] Implement `mdsmith.fixWorkspace`. Run
   `mdsmith fix .` from the workspace root. Parse
   the `stats:` line from stderr (see
   `printRunStats`). Show a notification with the
   fixed-of-total count. Cover with a VS Code
   extension test that mocks the child process.
3. [x] Implement `mdsmith.kinds.why` and
   `mdsmith.kinds.resolve` against the
   `mdsmith-kinds:` virtual document scheme.
   Register a `TextDocumentContentProvider` that
   returns the JSON output rendered as Markdown.
4. [x] Update the VS Code guide
   `docs/guides/editors/vscode.md` (created in
   plan 121) with one section per palette command.
5. [x] Add a CLI-subcommand table to
   `docs/guides/editors/vscode.md` listing every
   subcommand and its editor entry point. Mark
   `metrics`, `query`, and `version`
   as "CLI only".
6. [x] Wire workspace trust per the design above. Add
   the `capabilities.untrustedWorkspaces` block to
   `editors/vscode/package.json`. Gate the
   `fixWorkspace` and `mergeDriver.install` menu
   entries with `when: isWorkspaceTrusted`.
   Re-check `workspace.isTrusted` inside each
   handler. Subscribe to
   `onDidGrantWorkspaceTrust` to refresh the
   command surface without a reload.

## Acceptance Criteria

- [x] The command palette lists the five
      `mdsmith.*` commands in a workspace with a
      Markdown file open.
- [x] `mdsmith: Fix all Markdown` runs through a
      confirmation dialog, executes
      `mdsmith fix .`, and shows the fixed-count
      notification.
- [x] `mdsmith: Explain rule on this file` opens a
      virtual document with the kinds-why output
      for the selected rule.
- [x] No CodeLens, status-bar item, or
      activity-bar container is registered by the
      extension (verified by a CI grep on
      `editors/vscode/package.json`).
- [x] All tests pass: `go test ./...` and
      `npm test` inside `editors/vscode/`.
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes (subject to plan
      121's open question about `editors/**`
      exclusion).
- [x] Opening an untrusted workspace hides the
      `mdsmith.fixWorkspace` and
      `mdsmith.mergeDriver.install` palette
      entries; granting trust reveals them
      without a reload.
- [x] An untrusted workspace cannot redirect
      `mdsmith.path` or `mdsmith.config`; the
      extension uses the user-level value.

## ...

<?allow-empty-section?>
