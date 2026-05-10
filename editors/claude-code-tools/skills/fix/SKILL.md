---
name: fix
description: >-
  Auto-fix lint issues in Markdown files in place by
  shelling out to `mdsmith fix`. Use after editing
  Markdown across many files to clean up trailing
  whitespace, line length, bare URLs, generated
  sections, and other fixable rules in one pass.
user-invocable: true
allowed-tools: >-
  Bash(mdsmith fix:*), Bash(mdsmith fix --:*),
  Bash(npx -y -p @mdsmith/cli mdsmith fix:*)
argument-hint: "[path]"
---

Run `mdsmith fix` on the workspace (or a passed
path) and report the fix-of-total stats line.

## When to invoke

Invoke after Markdown edits that may have introduced
auto-fixable issues. The matching CLI reference is
[`mdsmith fix`](../../../../docs/reference/cli/fix.md).

## Steps

### 1. Resolve the target path

If the user passed a path argument, use it. Otherwise
default to `.` (the workspace root).

Note the value as `$TARGET`.

### 2. Run mdsmith fix

```bash
mdsmith fix -- "$TARGET"
```

The `--` terminator stops `$TARGET` from being parsed
as a flag if a filename starts with `-`.

If the binary is not on `$PATH`, fall back to:

```bash
npx -y -p @mdsmith/cli mdsmith fix -- "$TARGET"
```

### 3. Report results

`mdsmith fix` prints a `stats:` summary line that
lists files checked, fixed, failures, and unfixed
issues. Quote that line back to the user.

Exit 1 means at least one file still has unfixable
issues after the fix pass. Exit 2 means a runtime
or configuration error (bad path, unreadable
config, etc.) — the file may not have been touched
at all. Surface stderr in both cases so the user
sees the rule IDs and file locations, or the
config error.

## Notes

- Generated section bodies (between `<?directive ...?>`
  and `<?/directive?>` markers) are regenerated, not
  hand-edited — see [generated sections][gs].
- `mdsmith fix` writes in place. Stage or stash
  any unrelated work first if a clean diff matters.

[gs]: ../../../../docs/background/concepts/generated-section.md
