---
title: mdsmith reviewer plugin
summary: >-
  Marketplace plugin that ships the
  `markdown-reviewer` subagent for reviewing
  Markdown PRs and drafts in an mdsmith
  repository.
---
# mdsmith reviewer plugin

A Claude Code subagent that reviews Markdown
files for structural and organizational problems
beyond what `mdsmith check` enforces inline.

The agent loads rule-backed patterns from
`mdsmith help patterns` at review time — new
rules appear automatically without plugin
updates. It also checks three config-level
patterns from sibling `patterns.md`: missing
`.mdsmith.yml`, similar files without a kind,
and a kind without `path-pattern`. No auto-fix;
the agent proposes the config or directive to
adopt.

## Install

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-reviewer@mdsmith
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

Alternatively, the agent falls back to
`go run ./cmd/mdsmith` when the workspace
contains the mdsmith source tree.

## Usage

Invoke by name from any mdsmith-aware
repository:

```text
Review the Markdown changes in PR 42.
```

```text
Audit the docs/ directory for structural drift.
```

The agent produces a severity-grouped report
(blockers, tax, nice-to-have) with fix
snippets.

## What it checks

See
[`agents/markdown-reviewer.md`](agents/markdown-reviewer.md)
for the full workflow and
[`patterns.md`](patterns.md)
for the three config-level check recipes.
