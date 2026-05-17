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

Navigation comes from the same server. Jump to definition
resolves a link, anchor, reference label, directive argument,
or `kind:` value to its source. Find references lists every
use of a heading or label. The document outline lists a file's
headings, link-ref defs, and directives; workspace symbol
search spans the whole project.

Implementation jumps to every
target of a heading at once. Completion suggests anchors,
labels, kind names, and directive paths as you type. Hover
shows the rule's help on a diagnostic and the guide page on a
directive.

Two capabilities have their own pages. [Rename](rename.md)
rewrites a heading and every anchor link to it in one edit.
The [dependency graph](dependency-graph.md) walks
`<?include?>`, `<?catalog?>`, `<?build?>`, and cross-file
links as a call hierarchy — also available as `mdsmith deps`.

The [VS Code extension](../guides/editors/vscode.md) shows all of
it, with fix-on-save you can opt into. The same build ships on Open
VSX too. The Claude Code plugin feeds the same data to the agent.
