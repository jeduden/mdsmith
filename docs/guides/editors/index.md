---
title: Editors
summary: >-
  Editor integration guides for mdsmith — VS Code, Neovim, and
  Obsidian — all driven by the same bundled `mdsmith lsp` server.
---
# Editors

<?catalog
glob:
  - "*.md"
  - "!index.md"
sort: title
header: ""
row: "- [{title}]({filename}) — {summary}"
?>
- [mdsmith for Obsidian](obsidian.md) — Install the mdsmith Obsidian plugin and use its inline diagnostics, hover fixes, fix-on-save, and diagnostics panel — one WebAssembly runtime on desktop and mobile.
- [mdsmith for VS Code](vscode.md) — Install the mdsmith VS Code extension and use its inline diagnostics, quick fixes, fix-on-save, and cross-file navigation — one bundled binary, no extra setup.
- [Neovim Integration](neovim.md) — Wire `mdsmith lsp` into Neovim's built-in LSP client so diagnostics, code actions, and navigation work inline with no extra plugin.
<?/catalog?>
