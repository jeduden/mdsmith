---
title: "Live diagnostics wherever you write"
summary: >-
  `mdsmith lsp` emits diagnostics, quick-fixes, and navigation —
  definition, references, symbol search, and a call-hierarchy over
  `<?include?>`, `<?catalog?>`, and cross-file links — consumed by any
  LSP-aware editor.
icon: edit-3
link: "/docs/guides/editors/vscode/"
weight: 2
---
# Live diagnostics wherever you write

`mdsmith lsp` runs the same rule engine as the CLI over stdio,
speaking the Language Server Protocol. Any LSP-aware editor —
Neovim, Helix, or JetBrains via its LSP plugin — gets diagnostics
and quick-fixes inline.

Navigation comes from the same server. You get jump-to-source,
find-references, and symbol search. A call-hierarchy walks
`<?include?>`, `<?catalog?>`, and cross-file links.

The [VS Code extension](../guides/editors/vscode.md) shows all of
it, with fix-on-save you can opt into. The same build ships on Open
VSX too. The Claude Code plugin feeds the same data to the agent.
