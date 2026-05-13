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

## Goal

Apply every auto-fixable mdsmith rule to one path or
the whole workspace. Leave the corrected text on
disk. Report what was fixed, what still needs manual
attention, and any runtime error that aborted the
run.

## When to invoke

Invoke after Markdown edits that may have introduced
auto-fixable issues. The matching CLI reference is
[`mdsmith fix`](../../../../docs/reference/cli/fix.md).

## Steps

### 1. Resolve the target path

If the user passed a path argument, use it. Otherwise
use `.` (the workspace root). Note the value as
`$TARGET`. This step picks the file set the goal
operates over.

### 2. Run mdsmith fix to apply every fixable rule

```bash
mdsmith fix -- "$TARGET"
```

The `--` terminator keeps `$TARGET` parsed as a path
even when its name starts with `-`.

When the binary is missing from `$PATH`, fall back to
the npm-published variant:

```bash
npx -y -p @mdsmith/cli mdsmith fix -- "$TARGET"
```

### 3. Report what was fixed and what remains

`mdsmith fix` prints a `stats:` summary line that
lists files checked, fixed, failures, and unfixed
issues. Quote that line back to the user so they
see the win.

On exit 1, at least one file still has an unfixable
issue after the fix pass — surface stderr so the
user sees the rule IDs and file locations to address
by hand. On exit 2, treat the run as aborted by a
runtime or configuration error (bad path, unreadable
config, etc.) and surface stderr so the user sees
the cause.

## Notes

- Regenerate generated section bodies (between
  `<?directive ...?>` and `<?/directive?>` markers)
  by editing the directive parameters (or the source
  file the directive references) and re-running
  `mdsmith fix`. See [generated sections][gs].
- `mdsmith fix` writes in place. Stage or stash any
  unrelated work first when a clean diff matters.

[gs]: ../../../../docs/background/concepts/generated-section.md
