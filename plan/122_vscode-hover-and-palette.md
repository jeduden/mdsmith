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

Plan 121 covers diagnostics and per-file fixes.
Eight subcommands stay outside the editor:
`help`, `kinds`, `archetypes`, `metrics`,
`query`, `init`, `merge-driver`, `version`. A
reviewer audit grouped them:

- **Hover**: `help` (what does this rule mean?).
- **Palette**: `init`, `merge-driver install`,
  fix-everything, `kinds why`, `kinds resolve`.
- **CLI only**: `archetypes`, `metrics`, `query`,
  `version` — reviewers would uninstall a tree
  view or status-bar pill surfacing these.

## Design

### Hover

Capability: `hoverProvider = true`.

A `textDocument/hover` request arrives with a
position. When the position falls inside a
diagnostic range, the server returns Markdown:

- The diagnostic message (one line).
- The rule's docs, loaded the same way
  `mdsmith help rule <id|name>` loads them via
  [`internal/rules.LookupRule`](../internal/rules/ruledocs.go).

When no diagnostic covers the position, the server
checks for a `<?directive?>` block. If one is
under the cursor, the hover returns that
directive's docs, sourced from the existing files
under [docs/guides/directives/](../docs/guides/directives/).
This plan does not add a new `mdsmith help
directives` CLI topic; the directive content is
loaded directly by the LSP server.

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

### Workspace trust

The extension declares `capabilities.untrustedWorkspaces`
in `editors/vscode/package.json`:

```jsonc
"capabilities": {
  "untrustedWorkspaces": {
    "supported": "limited",
    "description": "Diagnostics and hover work in restricted mode. Commands that modify files outside the editor are disabled until the workspace is trusted.",
    "restrictedConfigurations": [
      "mdsmith.path",
      "mdsmith.config"
    ]
  }
}
```

- `"limited"` lets the language client and hover
  load in an untrusted workspace; neither writes
  files.
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

### Rule docs render in VS Code's Markdown engine

VS Code's hover renderer is Markdown-only and
strips inline HTML. The plan does not sanitize at
runtime. It enforces "no inline HTML" at lint
time on the rule README files. The vehicle is the
existing `rule-readme` kind in `.mdsmith.yml`.
That kind already targets
`internal/rules/MDS*/README.md`.

Today the kind only pins
`required-structure.schema`. The plan adds
MDS041 (`no-inline-html`). The rule is opt-in
by default. A settings mapping turns it on. There
is no `enabled` key — see
[`RuleCfg.UnmarshalYAML`](../internal/config/config.go):

```yaml
rule-readme:
  rules:
    required-structure:
      schema: internal/rules/proto.md
    no-inline-html:
      allow: [kbd]
```

A spot audit of
[`internal/rules/MDS*/README.md`](../internal/rules)
found no raw HTML outside code blocks. The
change is purely preventive. It stops a future
contributor from adding HTML the hover would
silently drop.

Editing `.mdsmith.yml` requires user consent per
[`CLAUDE.md`](../CLAUDE.md). The task surfaces
the diff first.

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

1. Wire hover's rule lookup through the existing
   [`internal/rules`](../internal/rules) APIs
   (`LookupRule(query) (string, error)` and
   `ListRules()`) so the hover body matches what
   `mdsmith help rule <id|name>` prints. Cover
   known rules, unknown rules, and rules whose
   docs have no rule-specific body.
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
8. Wire workspace trust per the design above. Add
   the `capabilities.untrustedWorkspaces` block to
   `editors/vscode/package.json`. Gate the
   `fixWorkspace` and `mergeDriver.install` menu
   entries with `when: isWorkspaceTrusted`.
   Re-check `workspace.isTrusted` inside each
   handler. Subscribe to
   `onDidGrantWorkspaceTrust` to refresh the
   command surface without a reload.
9. Propose enabling `no-inline-html` on the
   `rule-readme` kind in `.mdsmith.yml`. Surface
   the diff before applying. CLAUDE.md treats
   linter configuration as user-consent
   territory; this task lands the change only
   after authorisation. Add a regression test:
   a fixture rule README with a raw `<span>`
   outside a code block fails `mdsmith check`.

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
- [ ] Opening an untrusted workspace hides the
      `mdsmith.fixWorkspace` and
      `mdsmith.mergeDriver.install` palette
      entries; granting trust reveals them
      without a reload.
- [ ] An untrusted workspace cannot redirect
      `mdsmith.path` or `mdsmith.config`; the
      extension uses the user-level value.
- [ ] After the `rule-readme` kind change lands,
      a fixture rule README containing a raw
      `<span>` outside a code block fails
      `mdsmith check` with `MDS041`.

## ...

<?allow-empty-section?>
