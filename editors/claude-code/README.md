---
title: mdsmith for Claude Code
summary: >-
  Install the mdsmith Claude Code plugin so the agent
  receives Markdown diagnostics and navigation through
  the bundled LSP server.
---
# mdsmith for Claude Code

Inline mdsmith diagnostics and Markdown navigation
for Claude Code, wired through `mdsmith lsp`.

## Install

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-lsp@mdsmith
/reload-plugins
```

## Prerequisite

The plugin spawns the `mdsmith` binary from `$PATH`.
Install it first via any channel in the
[install guide](../../docs/guides/install.md) — npm,
PyPI, mise, or a GitHub release download.

## Troubleshooting

If the `/plugin` Errors tab shows `Executable not
found in $PATH`, the binary is missing from the
shell `$PATH` Claude Code sees. Reinstall via the
guide above, then run `/reload-plugins`.
