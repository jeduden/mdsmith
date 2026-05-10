---
name: kinds
description: >-
  Inspect declared file kinds and resolve the effective
  rule config for a file by shelling out to
  `mdsmith kinds`. Use to answer "what rules apply to
  this Markdown file?" or to debug why a particular
  rule did or did not fire.
user-invocable: true
allowed-tools: >-
  Bash(mdsmith kinds:*), Bash(mdsmith kinds --:*),
  Bash(npx -y -p @mdsmith/cli mdsmith kinds:*)
argument-hint: "[path]"
---

Show the kinds assigned to a file (or all files) and
the merged rule config that results.

## When to invoke

Invoke when a user asks which kind a file belongs to,
which rules apply to it, or why a rule's settings
differ from the project defaults. The matching CLI
reference is
[`mdsmith kinds`](../../../../docs/reference/cli/kinds.md).

## Steps

### 1. Pick the subcommand

If the user passed a path argument, run `kinds
resolve` against that file. Note the value as
`$TARGET` and use `kinds resolve`. Otherwise run
`kinds list` to print every declared kind with its
merged body, then ask the user which file they want
to dig into.

### 2. Run mdsmith kinds

For a single file:

```bash
mdsmith kinds resolve -- "$TARGET"
```

For the workspace-wide list:

```bash
mdsmith kinds list
```

If the binary is not on `$PATH`, prepend
`npx -y -p @mdsmith/cli ` to either command.

### 3. Read the output

`kinds resolve` lists the file's effective kinds plus
each rule key that differs from defaults, with
per-leaf provenance. `kinds list` lists every
declared kind. Surface the relevant block back to
the user verbatim.

Exit 1 means lint-style issues; exit 2 indicates a
runtime or configuration error (malformed
kind-assignment, missing config, etc.). Surface
stderr in both cases so the user sees the parse
error or rule context.

## Notes

- Kind-assignment is order-sensitive; later entries
  layer on earlier ones via deep merge. See
  [file-kinds.md](../../../../docs/guides/file-kinds.md).
