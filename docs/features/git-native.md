---
title: "Git-native, conflict-free"
summary: >-
  A Git merge driver auto-resolves conflicts inside generated
  blocks, and a pre-merge-commit hook re-runs `mdsmith fix` and
  re-stages the result, so generated content never blocks a merge.
icon: git-merge
link: "/reference/cli/merge-driver/"
weight: 16
---
# Git-native, conflict-free

Generated blocks are a merge magnet. Two branches both regenerate
a table of contents, and Git reports a conflict in content no
human wrote.

`mdsmith merge-driver install` registers a Git merge driver for
those blocks. On a conflict it re-runs the directive and keeps
the regenerated body, so the conflict never reaches you.

`mdsmith pre-merge-commit install` adds a hook that runs
`mdsmith fix` after a merge and re-stages the `.md` files. The
hook itself is kept honest by `MDS048`, which checks the Git
artifacts against the template derived from `.mdsmith.yml`.

See the [merge-driver](../reference/cli/merge-driver.md) and
[pre-merge-commit](../reference/cli/pre-merge-commit.md)
references for installation.
