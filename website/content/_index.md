---
title: "mdsmith"
summary: "A fast, auto-fixing Markdown linter and formatter for docs, READMEs, and AI-generated content. Checks style, readability, structure, and cross-file integrity. Written in Go."
hero:
  eyebrow: "Auto-fixing Markdown linter & formatter · Go"
  headline_pre: "Mark"
  headline_em: "down"
  headline_post: ", smithed."
  lead: >-
    Fast checks for style, readability, structure, and cross-file integrity.
    Auto-fix what fixes cleanly. Editor-grade diagnostics on every save.
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
  - id: claude
    label: "claude code"
    prompt: "/"
    cmd: "plugin marketplace add"
    args: "jeduden/mdsmith"
---
mdsmith is a Markdown linter and formatter written in Go. It checks style,
readability, structure, and cross-file integrity, and auto-fixes what fixes
cleanly. The same engine drives the CLI, the LSP server, the VS Code
extension, and the Claude Code plugin — every surface sees the same rules.
