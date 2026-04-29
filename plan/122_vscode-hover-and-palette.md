---
id: 122
title: VS Code hover help and palette commands
status: "🔲"
model: sonnet
summary: >-
  Add `textDocument/hover` to the LSP server so rule
  help text shows on hover over a diagnostic, plus a
  small set of VS Code command-palette entries —
  `init`, `merge-driver install`, `fix workspace`,
  `kinds why`, `kinds resolve` — that cover the
  remaining subcommands without adding chrome to the
  editor.
---
# VS Code hover help and palette commands

## Goal

After plan 121 ships diagnostics and code actions,
two questions stay unanswered from the editor:
"what does this rule mean?" and "how do I run a
mdsmith subcommand without leaving VS Code?". This
plan answers both with hover (for the first
question) and a short palette menu (for the
second).

The plan deliberately ships no permanent UI chrome.
No CodeLens, no status-bar item, no activity-bar
panel. Reviewers across personas (app dev, OSS
maintainer, SRE) all rated permanent UI as the
first feature they would disable.

## Background

Plan 121 covers diagnostics and per-file fix code
actions. It leaves eight subcommands without an
editor surface: `help`, `kinds`, `archetypes`,
`metrics`, `query`, `init`, `merge-driver`,
`version`. A reviewer audit grouped them by shape:

- Discovery during reading: `help` (what does this
  rule mean?). Right shape: hover.
- One-shot workspace actions: `init`,
  `merge-driver install`, fix-everything. Right
  shape: command palette.
- On-demand inspection: `kinds why`,
  `kinds resolve`. Right shape: palette command
  that opens a virtual document.
- Workspace browsing / ranking: `archetypes list`,
  `metrics rank`, `query`. Reviewers would
  uninstall a tree view that surfaced these. The
  CLI is the right shape; this plan does not wire
  them.
- Static info: `version`. Reviewers all flagged a
  status-bar pill as chrome. Out of scope.

## Design

### Hover

Capability: `hoverProvider = true`.

A `textDocument/hover` request arrives with a
position. When the position falls inside a
diagnostic range, the server returns Markdown:

- The diagnostic message (one line).
- The rule's `help` body, loaded the same way
  `mdsmith help <rule>` loads it.

When no diagnostic covers the position, the server
checks for a `<?directive?>` block. If one is
under the cursor, the hover returns that
directive's docs. The source is the same body
`mdsmith help directives` prints.

Hover does not link to `kinds why`. Reviewers
flagged that link as part of the over-surfaced
kinds chrome; the palette command below is the
single entry point.

### Palette commands

Each command is registered in
`editors/vscode/package.json` under
`contributes.commands` and bound to a handler in
`editors/vscode/src/commands/`.

| Command ID                    | Title                              | Action                                                   |
|-------------------------------|------------------------------------|----------------------------------------------------------|
| `mdsmith.init`                | mdsmith: Initialize config         | Run `mdsmith init` in the workspace root                 |
| `mdsmith.mergeDriver.install` | mdsmith: Install Git merge driver  | Run `mdsmith merge-driver install` after confirmation    |
| `mdsmith.fixWorkspace`        | mdsmith: Fix all Markdown          | Run `mdsmith fix .` against the workspace; show summary  |
| `mdsmith.kinds.why`           | mdsmith: Explain rule on this file | Pick a rule; open `mdsmith kinds why <file> <rule>` view |
| `mdsmith.kinds.resolve`       | mdsmith: Show resolved config      | Open `mdsmith kinds resolve <file> --json` virtual doc   |

Each handler spawns the binary, surfaces stderr in
a `mdsmith` output channel, and shows a
notification on non-zero exit. No status-bar
plumbing.

#### `mdsmith.fixWorkspace`

Fills the gap the SRE reviewer flagged. Per-buffer
`source.fixAll.mdsmith` cannot help when twenty
runbooks need the same trailing-space fix.

The handler runs `mdsmith fix .` from the
workspace root. It parses the `stats:` line. It
shows a notification: `Fixed 12 of 200 files`. A
"Show output" button opens the channel.

The command is gated by a confirmation dialog. It
touches files outside the active editor.
Restricted-trust workspaces skip the dialog and
fail closed.

#### `mdsmith.kinds.why` and `mdsmith.kinds.resolve`

Both handlers open a read-only virtual document
(`mdsmith-kinds:` URI scheme) populated from the
JSON output of the corresponding subcommand,
rendered as Markdown. No state, no refresh logic.
Closing the tab discards the buffer.

For `kinds.why`, the handler shows a quick-pick of
diagnostic rule IDs on the active editor; the
selection drives `mdsmith kinds why <file> <rule>`.

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

1. Expose a `Help(ruleID) (string, bool)` entry
   point on
   [`internal/rules`](../internal/rules) returning
   the same Markdown the CLI prints. Cover known
   rules, unknown rules, and rules with no help
   body.
2. Add `textDocument/hover` to the LSP server.
   Match the position against active diagnostic
   ranges. Fall through to the directive index
   when no diagnostic covers the cursor. Add an
   integration test in
   [`cmd/mdsmith`](../cmd/mdsmith) that drives the
   hover request and asserts the response.
3. Register the five palette commands in
   `editors/vscode/package.json`. Add one handler
   per command in `editors/vscode/src/commands/`.
   Each spawns the binary, surfaces stderr, and
   shows a notification on non-zero exit.
4. Implement `mdsmith.fixWorkspace`. Run
   `mdsmith fix .` from the workspace root. Parse
   the `stats:` line. Show a notification with the
   fixed-of-total count. Cover with a VS Code
   extension test that mocks the child process.
5. Implement `mdsmith.kinds.why` and
   `mdsmith.kinds.resolve` against the
   `mdsmith-kinds:` virtual document scheme.
   Register a `TextDocumentContentProvider` that
   returns the JSON output rendered as Markdown.
6. Update the VS Code guide
   `docs/guides/editors/vscode.md` (created in
   plan 121) with one section per surface: hover
   and each palette command.
7. Add a CLI-subcommand table to
   `docs/guides/editors/vscode.md` listing every
   subcommand and its editor entry point. Mark
   `archetypes`, `metrics`, `query`, and `version`
   as "CLI only".

## Acceptance Criteria

- [ ] Hovering over a `MDS001` squiggle in VS Code
      shows the line-length help body inline.
- [ ] Hovering inside a `<?catalog?>` directive
      shows the catalog directive docs even when no
      diagnostic is present.
- [ ] The command palette lists the five
      `mdsmith.*` commands in a workspace with a
      Markdown file open.
- [ ] `mdsmith: Fix all Markdown` runs through a
      confirmation dialog, executes
      `mdsmith fix .`, and shows the fixed-count
      notification.
- [ ] `mdsmith: Explain rule on this file` opens a
      virtual document with the kinds-why output
      for the selected rule.
- [ ] No CodeLens, status-bar item, or
      activity-bar container is registered by the
      extension (verified by a CI grep on
      `editors/vscode/package.json`).
- [ ] All tests pass: `go test ./...` and
      `npm test` inside `editors/vscode/`.
- [ ] `go tool golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes (subject to plan
      121's open question about `editors/**`
      exclusion).

## Open Questions

- **Workspace trust for fixWorkspace and
  mergeDriver.install.** Both write outside the
  active buffer. Pick the right
  `untrustedWorkspaces` capability flag during
  implementation.
- **Markdown rendering inside hovers.** VS Code
  renders LSP hover Markdown but strips inline
  HTML. Confirm the rule-help bodies do not rely
  on raw HTML; route through a sanitizer if any
  do.

## ...

<?allow-empty-section?>
