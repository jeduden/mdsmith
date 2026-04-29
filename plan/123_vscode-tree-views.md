---
id: 123
title: Workspace tree views for kinds, archetypes, and metrics
status: "🔲"
model: sonnet
summary: >-
  Add three VS Code activity-bar panels backed by
  `mdsmith kinds list --json`, `mdsmith archetypes
  list --json`, and `mdsmith metrics rank --json` so
  workspace-wide views surface without the user
  shelling out.
---
# Workspace tree views for kinds, archetypes, and metrics

## Goal

Surface mdsmith's workspace-wide subcommands as
clickable tree views in VS Code. A user opens the
mdsmith activity-bar icon and sees three panels:
`Kinds`, `Archetypes`, and `Metrics`. Each panel
mirrors the JSON output of the corresponding CLI
subcommand and refreshes when `.mdsmith.yml`
changes.

## Background

Three subcommands describe the workspace rather
than a single file:

- `mdsmith kinds list` enumerates declared kinds
  and their merged bodies.
- `mdsmith archetypes list` enumerates archetype
  schemas discovered under
  `archetypes.roots`.
- `mdsmith metrics rank` ranks files by a chosen
  metric.

Hover and CodeLens (plan 122) cover the per-file
cases. The list / rank cases need a different
shape: a collapsible tree with one row per kind,
archetype, or file. VS Code's `TreeDataProvider`
API is the standard wire for this, and each
subcommand already emits stable JSON for tool
consumption.

## Design

### Activity-bar container

The extension contributes one activity-bar
container named `mdsmith`. It holds three views.
Each view has its own `TreeDataProvider`:

| View         | Backing command                        |
|--------------|----------------------------------------|
| `Kinds`      | `mdsmith kinds list --json`            |
| `Archetypes` | `mdsmith archetypes list --json`       |
| `Metrics`    | `mdsmith metrics rank --json --top 50` |

Each provider runs the binary in a child process
and parses stdout. Stderr is forwarded to the
extension's output channel.

### Kinds view

Top-level rows are kinds. Expanding a kind shows
one row per rule with the merged settings. Clicking
a rule row runs `mdsmith kinds why <file> <rule>`
against the active editor and opens the result as a
virtual document. Right-click context menu adds
`Open schema` for kinds whose `required-structure`
schema resolves to a file.

### Archetypes view

Top-level rows are archetypes. Each row shows the
archetype name and resolved path. Clicking opens
the schema source in a read-only editor (the same
content `mdsmith archetypes show <name>` prints).

### Metrics view

Top-level rows are files, ranked by the configured
metric. The view has a header row with two
controls:

- **Metric**: dropdown driven by `mdsmith metrics
  list --json`.
- **Order**: `asc` / `desc`.

Changing either control re-runs `mdsmith metrics
rank` with the new flags and refreshes the tree.
Clicking a file row opens it in the editor and
jumps to the line that drove the score when the
metric reports one.

### Refresh

A single `FileSystemWatcher` on `**/.mdsmith.yml`
fires `refresh` on all three providers. The
extension also exposes a `mdsmith.refreshViews`
command bound to a refresh button on each panel
title bar.

### CLI changes

The CLI already supports `--json` on `kinds list`
and `archetypes list`. `metrics rank` currently
accepts `--format=json`; this plan does not change
the CLI. If `metrics list` does not yet take
`--json`, add it as a small precursor task — the
view needs the metric catalogue as structured
data.

## Tasks

1. Audit `metrics list` and `metrics rank` JSON
   shapes for the fields the tree view needs
   (`name`, `description`, `file`, `score`,
   optional `line`). Add any missing field with a
   minimal CLI test.
2. Add `editors/vscode/src/views/kinds.ts` with a
   `TreeDataProvider` that calls `mdsmith kinds
   list --json` and renders the two-level tree.
3. Add `editors/vscode/src/views/archetypes.ts`
   with a provider for `mdsmith archetypes list
   --json` plus an `Open schema` command bound to
   `mdsmith archetypes show --raw`.
4. Add `editors/vscode/src/views/metrics.ts` with a
   provider for `mdsmith metrics rank --json`. Wire
   the metric and order controls; persist the
   selected metric per workspace via
   `Memento`.
5. Register the `mdsmith` view container and the
   three views in
   `editors/vscode/package.json`. Add a refresh
   command, a refresh button per view, and a
   `markdown.icon` for the activity bar.
6. Wire a single `FileSystemWatcher` on
   `**/.mdsmith.yml` that calls `refresh` on each
   provider.
7. Add VS Code extension tests that mock the child
   process and assert each provider renders the
   expected tree shape from a fixture JSON
   payload.
8. Update the VS Code guide
   `docs/guides/editors/vscode.md` with one section
   per view, including a note that the metrics
   view counts only authored bytes (the
   `metrics rank` lint-once contract).

## Acceptance Criteria

- [ ] The mdsmith activity-bar container shows
      three views: `Kinds`, `Archetypes`,
      `Metrics`.
- [ ] The kinds view top level matches `mdsmith
      kinds list --json` byte-for-byte (same names,
      same order).
- [ ] Clicking a rule in the kinds view opens the
      `kinds why` virtual document for that rule
      and the active editor.
- [ ] The archetypes view lists every archetype
      `mdsmith archetypes list` reports and opens
      the schema source on click.
- [ ] The metrics view re-runs `mdsmith metrics
      rank` when the metric or order is changed and
      jumps to the reported line on click.
- [ ] Saving an edit to `.mdsmith.yml` refreshes
      every view within one second.
- [ ] All tests pass: `go test ./...` and
      `npm test` inside `editors/vscode/`.
- [ ] `go tool golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.

## Open Questions

- **Empty-state copy.** Each view needs a message
  for the empty case (no kinds declared, no
  archetype roots configured, no files ranked).
  Decide on the wording during implementation.
- **Per-workspace metric persistence.** `Memento`
  scopes well, but a user with many workspaces may
  prefer a global default. Add a setting only if a
  user asks for it.

## ...

<?allow-empty-section?>
