---
title: mdsmith tools for Claude Code
summary: >-
  Install the mdsmith-tools Claude Code plugin to add
  Markdown skills, a reviewer subagent, and a post-edit
  lint hook on top of the LSP-only mdsmith-lsp plugin.
---
# mdsmith tools for Claude Code

Skills, a Markdown reviewer agent, and a post-edit
lint hook for Claude Code. Pairs with `mdsmith-lsp`
but does not require it.

## Install

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-tools@mdsmith
/reload-plugins
```

## Components

| Type  | Name                                        | Purpose                          |
|-------|---------------------------------------------|----------------------------------|
| Skill | `/mdsmith-tools:fix`                        | Run `mdsmith fix .` on workspace |
| Skill | `/mdsmith-tools:kinds`                      | Show kind assignments + config   |
| Skill | `/mdsmith-tools:check`                      | Run `mdsmith check .`            |
| Agent | `markdown-reviewer`                         | Review Markdown PRs and drafts   |
| Hook  | `PostToolUse` on `Edit`/`Write` of Markdown | Auto-run `mdsmith fix` per file  |

## Prerequisites

Each component shells out to `mdsmith`. Install
globally with `npm i -g @mdsmith/cli`. Node 18+
(with npm) must be on `$PATH`.

The post-edit hook also depends on `jq` to parse
the tool-input JSON Claude Code sends on stdin.
`jq` ships in most distros and Homebrew; install it
with `apt`, `brew`, or your package manager.

## Hook scope

The post-edit hook fires on `Edit` and `Write` of
`*.md` and `*.markdown` files. `MultiEdit` is
intentionally skipped — use the LSP
`source.fixAll.mdsmith` action from `mdsmith-lsp`
for multi-buffer fixes.

Disable the plugin if you prefer to run `fix`
manually.
