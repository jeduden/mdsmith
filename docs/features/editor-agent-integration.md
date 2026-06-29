---
title: "Editors and agents"
summary: >-
  A VS Code extension and Claude Code plugins run the same rule
  engine, so diagnostics, quick-fixes, and navigation reach your
  editor and your agent.
icon: plug
link: "/guides/editors/vscode/"
weight: 4
group: "One engine, every surface"
---
# Editors and agents

mdsmith runs one rule engine everywhere. The check that gates a
merge in CI is the same one your editor shows you. Your coding
agent sees that same check as it edits your Markdown. No second
config, no second ruleset.

<?include
file: ../brand/messaging.md
extract: vscode-overview.text
?>
The extension is a thin LSP client over the bundled mdsmith binary, which it runs with the lsp subcommand. Diagnostics appear inline as squiggles, and every fixable rule contributes a lightbulb quick fix. A whole-buffer fix action runs on demand or on save, with an optional Refactor Preview before edits land. Cross-file navigation extends to Go to Definition, Find All References, workspace symbol search, and a call hierarchy across includes, catalogs, builds, and Markdown links. The mdsmith Command Palette runs Initialize Config, Fix All Markdown, Install Git Merge Driver, Explain Rule on This File, and Show Resolved Config. The .vsix bundles the mdsmith binary for every supported OS and architecture, so no separate install is needed.
<?/include?>

The same `.vsix` ships on Open VSX, so Cursor, VSCodium, Theia,
and Gitpod install it too. The
[Obsidian plugin](../guides/editors/obsidian.md) runs the engine
as WebAssembly, on desktop and mobile.

In Claude Code, mdsmith shows the agent the same diagnostics CI
runs: broken links, missing anchors, and heading or schema
violations. The agent catches and fixes them as it writes, before
the change reaches your review.

The marketplace ships five plugins. `mdsmith-lsp` gives the agent
inline diagnostics and the navigation the editor gets.
`mdsmith-autofix` runs `mdsmith fix` after every Markdown edit, so
generated sections and formatting stay correct with no manual
step. Three slash commands run the CLI from the prompt:
`/mdsmith-fix`, `/mdsmith-check`, and `/mdsmith-kinds`. A
`markdown-reviewer` subagent reviews Markdown PRs and drafts for
structural drift. The `/markdown-audit` skill audits a
repository's file layout: hand-kept indexes, missing kinds, and
absent schemas.

Register the marketplace once, then install the plugins you want:

```text
/plugin marketplace add jeduden/mdsmith
```

The [install guide](../guides/install.md#claude-code-plugins)
lists the per-plugin install commands and prerequisites. mdsmith
makes no network call of its own; each plugin only runs the local
binary (see the [telemetry policy](../reference/telemetry.md)).

Pair the plugins with
[progressive disclosure](../guides/progressive-disclosure.md). A
`<?catalog?>` in `CLAUDE.md` keeps one summary line per tracked
doc. The agent reads that index up front, then opens only the
files a task touches.

See the [VS Code guide](../guides/editors/vscode.md) and the
[Obsidian guide](../guides/editors/obsidian.md) for editor setup.
