---
title: "mdsmith"
summary: "A fast, auto-fixing Markdown linter and formatter. Checks style, readability, structure, and cross-file integrity. Written in Go, MIT-licensed, no telemetry."
hero:
  eyebrow: "Auto-fixing Markdown linter & formatter · Go"
  headline_pre: "Mark"
  headline_em: "down"
  headline_post: ", smithed."
  lead: >-
    Fast checks for style, readability, structure, and cross-file integrity.
    One static Go binary, no runtime — [roughly 4× faster than Node
    markdownlint on a 700-file corpus](features/performance/). Auto-fix
    what fixes cleanly. Editor-grade diagnostics on every save.
install:
  - id: go
    label: "go"
    prompt: "$"
    cmd: "go install"
    args: "github.com/jeduden/mdsmith/cmd/mdsmith@latest"
  - id: npm
    label: "npm"
    prompt: "$"
    cmd: "npm install -g"
    args: "@mdsmith/cli"
  - id: pip
    label: "pip"
    prompt: "$"
    cmd: "pip install"
    args: "mdsmith"
  - id: vscode
    label: "vs code"
    prompt: "$"
    cmd: "code --install-extension"
    args: "jeduden.mdsmith"
  - id: neovim
    label: "neovim"
    prompt: "$"
    cmd: "mdsmith lsp"
    args: ""
  - id: claude
    label: "claude code"
    prompt: "/"
    cmd: "plugin marketplace add"
    args: "jeduden/mdsmith"
---
mdsmith is a Markdown linter and formatter written in Go. It checks style,
readability, structure, and cross-file integrity, and auto-fixes what fixes
cleanly. Where markdownlint-compatible linters stop at per-file style,
mdsmith adds the cross-file graph, generated sections, and readability
budgets. One rule engine powers the CLI, the LSP server, and the VS Code
extension — Neovim and other LSP-aware editors plug in through the same
server, and a Claude Code plugin is available for users of that editor.
