---
title: Claude Code dev plugin
summary: >-
  Contributor-only Claude Code plugin that wires
  Go (gopls) and TypeScript LSP servers so any
  agent working on the mdsmith codebase gets
  code intelligence across cmd/, internal/, and
  editors/vscode/.
---
# Claude Code dev plugin

This directory ships the `mdsmith-dev-lsp`
plugin. It is contributor-facing and
self-contained: it declares the three
language servers an agent needs while
working on this repo.

Project settings auto-enable the plugin
from the local marketplace. The `LSP`
tool then sees these files:

- Go under `cmd/` and `internal/` via
  `gopls`.
- TypeScript under `editors/vscode/` via
  `typescript-language-server` over `npx`.
- Markdown across the whole repo via the
  stable `@mdsmith/cli` LSP server, also
  fetched through `npx` — same invocation
  as the published `mdsmith-lsp` plugin so
  the repo's own Markdown gets linted with
  the released ruleset.

See `.claude/settings.json` and
`.claude-plugin/marketplace.json` for the
wiring.

## Prerequisites

- `gopls` on `$PATH`. Install with
  `go install golang.org/x/tools/gopls@latest`.
- A working `npx` (any recent Node.js).
  The TypeScript LSP and `@mdsmith/cli` are
  fetched lazily on first use; no global
  install needed.

## Why a separate plugin

The published `mdsmith-lsp` plugin under
`editors/claude-code/` ships only the
Markdown LSP. End users want Markdown
intelligence in their own projects.
Bundling `gopls` and
`typescript-language-server` into that
plugin would spawn extra processes for
every end user.

This dev plugin keeps that cost on
contributors. It still pulls the
Markdown LSP from the stable npm
release, so the repo's Markdown lints
against the released ruleset.
