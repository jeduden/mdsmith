---
title: mdsmith autofix plugin
summary: >-
  Marketplace plugin that installs a PostToolUse
  hook running `mdsmith fix` on every `.md` file
  Claude Code edits.
---
# mdsmith autofix plugin

A Claude Code hook that runs `mdsmith fix`
automatically after every `Edit`, `Write`, or
`MultiEdit` tool call on a `.md` or `.markdown`
file.

The hook keeps generated sections (catalog,
include, toc, build), whitespace, headings, and
table alignment in sync as the agent edits.
Without this plugin, `mdsmith fix` must be run
manually or via `/mdsmith-fix`.

## Install

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-autofix@mdsmith
/reload-plugins
```

## Prerequisites

`mdsmith` and `jq` must both be on the `$PATH`
that Claude Code sees.

Install `mdsmith`:

```text
npm install -g @mdsmith/cli
```

Install `jq` via your OS package manager (e.g.
`brew install jq`, `apt install jq`).

## Opt-out

This plugin is opt-in. If you prefer to run
`mdsmith fix` manually or via `/mdsmith-fix`,
simply do not install `mdsmith-autofix`.

## Hook details

The hook fires on `PostToolUse` for
`Edit|Write|MultiEdit`. It reads the edited
file path from `.tool_input.file_path`, strips
the workspace prefix so relative globs match,
and calls `mdsmith fix -- <relative-path>`.

Non-Markdown files are skipped silently. A
`mdsmith fix` failure does not fail the hook —
it exits 0 so the agent is not blocked.
