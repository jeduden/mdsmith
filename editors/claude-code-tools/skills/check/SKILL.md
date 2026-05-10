---
name: check
description: >-
  Lint Markdown files for style issues by shelling out
  to `mdsmith check`. Use this when the LSP plugin
  (mdsmith-lsp) is not installed, or when you need a
  workspace-wide pass rather than per-file diagnostics.
user-invocable: true
allowed-tools: >-
  Bash(mdsmith check:*), Bash(mdsmith check --:*),
  Bash(npx -y -p @mdsmith/cli mdsmith check:*)
argument-hint: "[path]"
---

Run `mdsmith check` on the workspace (or a passed
path) and surface diagnostics to the user.

## When to invoke

Invoke for a one-shot workspace lint pass, or to
verify a fix worked. For inline per-edit diagnostics
prefer the `mdsmith-lsp` plugin instead — the LSP
streams updates without re-running the whole repo.

The matching CLI reference is
[`mdsmith check`](../../../../docs/reference/cli/check.md).

## Steps

### 1. Resolve the target path

If the user passed a path argument, use it. Otherwise
default to `.` (the workspace root).

Note the value as `$TARGET`.

### 2. Run mdsmith check

```bash
mdsmith check -- "$TARGET"
```

The `--` terminator stops `$TARGET` from being parsed
as a flag if a filename starts with `-`.

If the binary is not on `$PATH`, fall back to:

```bash
npx -y -p @mdsmith/cli mdsmith check -- "$TARGET"
```

### 3. Report results

`mdsmith check` prints one line per diagnostic, then
a `stats:` summary line. Surface the diagnostics
verbatim grouped by file so the user can navigate to
each `path:line:col` location.

Exit 1 means at least one diagnostic was emitted.
Exit 2 indicates a runtime or configuration error
(unreadable file, bad config, etc.) and the lint
pass did not run — surface stderr so the user
sees the underlying error. Do not suppress stderr
in either case.

## Notes

- To auto-fix a subset of these diagnostics in the
  same pass, follow up with `/mdsmith-tools:fix`.
- Pre-existing failures unrelated to the user's
  current edits should be flagged as such, not
  silently fixed.
