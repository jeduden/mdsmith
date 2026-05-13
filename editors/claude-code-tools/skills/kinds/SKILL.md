---
name: kinds
description: >-
  Inspect declared file kinds and resolve the effective
  rule config for a file by shelling out to
  `mdsmith kinds`. Use to answer "what rules apply to
  this Markdown file?" or to debug why a particular
  rule fired or stayed silent.
user-invocable: true
allowed-tools: >-
  Bash(mdsmith kinds:*), Bash(mdsmith kinds --:*),
  Bash(npx -y -p @mdsmith/cli mdsmith kinds:*)
argument-hint: "[path]"
---

## Goal

Tell the user which kinds are assigned to a given
Markdown file, or list every declared kind across the
workspace. Surface the merged rule config with
per-leaf provenance, so they can answer "what rules
apply here?" and "where did this setting come from?".

## When to invoke

Invoke when a user asks which kind a file belongs to,
which rules apply to it, or why a rule's settings
differ from the project defaults. The matching CLI
reference is
[`mdsmith kinds`](../../../../docs/reference/cli/kinds.md).

## Steps

### 1. Pick the subcommand that fits the question

If the user passed a path argument, run `kinds
resolve` against that file and note the value as
`$TARGET` — `resolve` answers "what applies to this
file?". Otherwise run `kinds list` to enumerate every
declared kind, then ask the user which file to dig
into next.

### 2. Run mdsmith kinds

For a single file:

```bash
mdsmith kinds resolve -- "$TARGET"
```

For the workspace-wide list:

```bash
mdsmith kinds list
```

When the binary is missing from `$PATH`, prepend
`npx -y -p @mdsmith/cli ` to either command.

### 3. Surface the merged config to the user

`kinds resolve` lists the file's effective kinds plus
each rule key that differs from defaults, with
per-leaf provenance. `kinds list` enumerates every
declared kind. Quote the relevant block back to the
user verbatim so they can read the provenance trail.

`mdsmith kinds` exits 0 on success or exit 2 on a
runtime/configuration error (unknown kind, unreadable
file, malformed `.mdsmith.yml`, etc.). On exit 2,
surface stderr so the user sees the parse or load
error.

## Notes

- Kind-assignment is order-sensitive; later entries
  layer on earlier ones via deep merge. See
  [file-kinds.md](../../../../docs/guides/file-kinds.md).
