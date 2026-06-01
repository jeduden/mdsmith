---
title: "mdsmith"
summary: "Write content; mdsmith keeps your Markdown neat and consistent — fast enough to stay out of your way. Auto-fix on save, instant navigation, cross-file integrity, and generated sections that keep a single source of truth in sync across files and pipelines."
hero:
  eyebrow: "Markdown as a single source of truth"
  headline_pre: "Mark"
  headline_em: "down"
  headline_post: ", smithed."
  lead: "Write content; mdsmith keeps your Markdown neat and consistent — fast enough to stay out of your way. Auto-fix on save, instant navigation, cross-file integrity, and generated sections that keep derived data in sync, so the same Markdown drives docs, READMEs, and downstream pipelines without drift."
---
mdsmith is a Markdown linter and formatter written in Go. It checks style,
readability, structure, and cross-file integrity, and auto-fixes what fixes
cleanly. Where markdownlint-compatible linters stop at per-file style,
mdsmith adds the cross-file graph, generated sections, and readability
budgets. One rule engine powers the CLI, the LSP server, and the VS Code
extension — Neovim and other LSP-aware editors plug in through the same
server, and a Claude Code plugin is available for users of that editor.
