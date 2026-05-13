---
name: check
description: >-
  Lint Markdown files for style issues by shelling out
  to `mdsmith check`. Use this when the LSP plugin
  (mdsmith-lsp) is unavailable, or when you need a
  workspace-wide pass instead of per-file diagnostics.
user-invocable: true
allowed-tools: >-
  Bash(mdsmith check:*), Bash(mdsmith check --:*),
  Bash(npx -y -p @mdsmith/cli mdsmith check:*)
argument-hint: "[path]"
---

## Goal

Run a one-shot mdsmith lint pass over the requested
Markdown set. Present every diagnostic with file
path, line, column, rule ID, and message. Help the
user pick which to fix, find where, and tell
pre-existing issues from ones introduced this
session.

## When to invoke

Invoke for a one-shot workspace lint pass, or to
verify a fix worked. For inline per-edit diagnostics,
prefer the `mdsmith-lsp` plugin instead — the LSP
streams updates as the user edits.

The matching CLI reference is
[`mdsmith check`](../../../../docs/reference/cli/check.md).

## Steps

### 1. Resolve the target path

If the user passed a path argument, use it. Otherwise
use `.` (the workspace root). Note the value as
`$TARGET`. This step picks the file set the goal
covers.

### 2. Run mdsmith check

```bash
mdsmith check -- "$TARGET"
```

The `--` terminator keeps `$TARGET` parsed as a path
even when its name starts with `-`.

When the binary is missing from `$PATH`, fall back to
the npm-published variant:

```bash
npx -y -p @mdsmith/cli mdsmith check -- "$TARGET"
```

### 3. Surface every diagnostic to the user

`mdsmith check` prints one line per diagnostic, then a
`stats:` summary line. Quote the diagnostics verbatim
grouped by file so the user can navigate to each
`path:line:col` location.

On exit 1, the lint pass produced at least one
diagnostic — quote them back to the user. On exit 2,
the lint pass aborted before producing diagnostics
because of a runtime or configuration error
(unreadable file, bad config, etc.). Surface stderr
in both cases so the user sees the rule context or
the underlying error.

## Notes

- To auto-fix a subset of the diagnostics in the same
  pass, follow up with `/mdsmith-tools:fix`.
- Label diagnostics that pre-date the current edits
  as "pre-existing" in your report so the user can
  tell them apart from issues introduced this session.
