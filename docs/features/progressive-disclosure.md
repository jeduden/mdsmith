---
title: "Agent-ready docs index"
summary: >-
  A `<?catalog?>` in `CLAUDE.md` keeps one `summary` line per
  tracked doc, so an agent reads a few thousand tokens of
  metadata up front and opens only the files a task touches.
icon: list-tree
link: "/guides/progressive-disclosure/"
weight: 13
group: "Markdown as a single source of truth"
---
# Agent-ready docs index

AI coding agents pay context tokens for every file they read.
Loading a whole docs tree up front wastes most of them.

Progressive disclosure inverts that. A `<?catalog?>` in the
agent file keeps one `summary` line per tracked doc. The agent
skims the index, then opens only the files a task touches.
`mdsmith check` flags the index when it drifts; `mdsmith fix`
rebuilds it.

mdsmith's own [`CLAUDE.md`](../../CLAUDE.md) is the live
example. Its catalog compresses the docs tree to a few thousand
tokens of metadata.

See the [progressive disclosure
guide](../guides/progressive-disclosure.md) for the step-by-step
recipe.
