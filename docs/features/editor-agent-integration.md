---
title: "Editors and agents"
summary: >-
  A bundled VS Code extension and Claude Code plugins drive the same
  `mdsmith lsp` server, so diagnostics, fix-on-save, and navigation
  reach your editor and your coding agent unchanged.
icon: plug
link: "/guides/editors/vscode/"
weight: 5
group: "One engine, every surface"
---
# Editors and agents

The rule engine is the same everywhere. The value is getting it
into the tools you already use without a separate config.

<?include
file: ../brand/messaging.md
extract: vscode-overview.text
?>
The extension is a thin LSP client over the bundled mdsmith binary, which it runs with the lsp subcommand. Diagnostics appear inline as squiggles, and every fixable rule contributes a lightbulb quick fix. A whole-buffer fix action runs on demand or on save, with an optional Refactor Preview before edits land. Cross-file navigation extends to Go to Definition, Find All References, workspace symbol search, and a call hierarchy across includes, catalogs, builds, and Markdown links. The mdsmith Command Palette runs Initialize Config, Fix All Markdown, Install Git Merge Driver, Explain Rule on This File, and Show Resolved Config. The .vsix bundles the mdsmith binary for every supported OS and architecture, so no separate install is needed.
<?/include?>

The same `.vsix` is published to Open VSX, so Cursor, VSCodium,
Theia, and Gitpod install it too.

The Claude Code plugin marketplace ships `mdsmith-lsp`, which
feeds the same diagnostics and navigation to the agent, plus a
Markdown-organization audit skill. The agent sees mdsmith inline
while it edits your docs.

See the [VS Code guide](../guides/editors/vscode.md), the
[Obsidian guide](../guides/editors/obsidian.md), and the
[install guide](../guides/install.md) for setup.
