---
title: "See the dependency graph"
summary: >-
  `mdsmith deps` lists what a file pulls in — includes, catalogs,
  build sources, and links — or, with `--incoming`, every file that
  points at it. The LSP call-hierarchy walks the same graph in your
  editor.
icon: network
weight: 16
link: "/reference/cli/deps/"
---
# See the dependency graph

mdsmith already tracks every cross-file edge: `<?include?>`,
`<?catalog?>`, `<?build?>`, and Markdown links. The same
graph that powers cross-file integrity checks answers two
questions directly.

To see what a file pulls in, ask for its outgoing edges:

```bash
mdsmith deps docs/index.md
```

To see what depends on a file — run this before you move
or delete a page, so a refactor never strands an include
or a link — ask for its incoming edges:

```bash
mdsmith deps docs/api.md --incoming
```

Output is one row per edge, in text or JSON, with stable
keys for CI and agents. Exit codes follow the usual
convention: `0` when edges exist, `1` when none, `2` on
error.

The editor surface is the same graph. `mdsmith lsp`
exposes it as a call hierarchy: walk incoming and outgoing
edges over includes, catalogs, builds, and links without
leaving the file. The CLI and the LSP read one index, so a
dependency the command prints is the dependency the editor
navigates.

See the [`mdsmith deps` reference](../reference/cli/deps.md)
and the [LSP reference](../reference/cli/lsp.md).
