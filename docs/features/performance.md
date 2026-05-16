---
title: "Fast on every run"
summary: >-
  A single static Go binary with no runtime to boot. The workspace
  walk runs in parallel, embeds are linted once, and `check` is built
  for the hot path so CI and editor feedback stay instant.
icon: zap
link: "/docs/reference/cli/check/"
weight: 7
---
# Fast on every run

Speed is a feature, not an afterthought. mdsmith ships as one
static Go binary. There is no Node or Python runtime to start, so
the first run is as fast as the next.

The workspace walk fans out across files in parallel. Files
pulled in by `<?include?>` and `<?catalog?>` are linted once, not
once per host, so a large doc set does not re-scan the same prose.

`mdsmith check` is the hot path. It shares the rule engine with
the LSP server and the fixer, so editor feedback and CI use the
same fast core. A check over this repository runs in well under a
second.

See the [`check`](../reference/cli/check.md) reference for flags
and exit codes.
