---
title: mdsmith skills plugin
summary: >-
  Marketplace plugin that ships slash-command
  skills for `mdsmith fix`, `mdsmith kinds`, and
  `mdsmith check`.
---
# mdsmith skills plugin

Three Claude Code slash-command skills that run
`mdsmith` subcommands from the agent.

## Install

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-skills@mdsmith
/reload-plugins
```

## Prerequisites

`mdsmith` must be on the `$PATH` that Claude
Code sees. Install it via:

```text
npm install -g @mdsmith/cli
```

or any other channel from the
[install guide](../../docs/guides/install.md).

Alternatively, the skills fall back to
`go run ./cmd/mdsmith` when the workspace
contains the mdsmith source tree.

## Skills

| Slash command                             | What it runs    |
|-------------------------------------------|-----------------|
| `/mdsmith-fix [path]`                     | `mdsmith fix`   |
| `/mdsmith-kinds [resolve <file> \| list]` | `mdsmith kinds` |
| `/mdsmith-check [path]`                   | `mdsmith check` |

All three default to `.` (workspace root) when
no argument is given.

See each `SKILL.md` under `skills/` for the
full workflow.
