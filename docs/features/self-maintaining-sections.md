---
title: "Self-maintaining sections"
summary: >-
  On `mdsmith fix`, `<?toc?>` rebuilds a heading TOC, `<?catalog?>`
  generates an index from front matter, and `<?include?>` splices in
  another file. A Git merge driver auto-resolves conflicts inside
  those blocks.
icon: list-checks
link: "/guides/directives/generating-content/"
weight: 12
group: "Markdown as a single source of truth"
---
# Self-maintaining sections

Some sections should never be hand-edited. mdsmith marks them
with directives and rebuilds the body on `mdsmith fix`.

`<?toc?>` rebuilds a heading table of contents. `<?catalog?>`
generates an index — a list, a table, or any row template — from
the front matter of files matching a glob. `<?include?>` splices
in another file.

Generated blocks fight Git merges. `mdsmith merge-driver install`
registers a driver for them. It re-runs the directive and resolves
the conflict for you.

See the [generating-content
guide](../guides/directives/generating-content.md) for directive
syntax and options.
