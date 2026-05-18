---
title: "Editors and agents"
summary: >-
  A bundled VS Code extension and Claude Code plugins drive the same
  `mdsmith lsp` server, so diagnostics, fix-on-save, and navigation
  reach your editor and your coding agent unchanged.
icon: plug
link: "/guides/editors/vscode/"
weight: 14
---
# Editors and agents

The rule engine is the same everywhere. The value is getting it
into the tools you already use without a separate config.

The VS Code extension ships diagnostics, opt-in fix-on-save, and
commands for init, the merge driver, fix-all, and rule explain.
The same `.vsix` is published to Open VSX, so Cursor, VSCodium,
Theia, and Gitpod install it too.

The Claude Code plugin marketplace ships `mdsmith-lsp`, which
feeds the same diagnostics and navigation to the agent, plus a
Markdown-organization audit skill. The agent sees mdsmith inline
while it edits your docs.

See the [VS Code guide](../guides/editors/vscode.md) and the
[install guide](../guides/install.md) for setup.
