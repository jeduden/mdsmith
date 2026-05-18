---
name: mdsmith-kinds
description: >-
  Inspect how mdsmith resolves file kinds in the
  current workspace. `resolve` shows the effective
  kind and merged rule config for one file;
  `list` enumerates all declared kinds. Trigger
  when the user asks "what kind is this file",
  "which rules apply here", "show me the kinds",
  or "resolve this file's config".
user-invocable: true
argument-hint: "[resolve <file> | list]"
allowed-tools: "Bash(mdsmith:*)"
---
# mdsmith kinds

Inspect file-kind resolution and the effective
rule config that results.

## Subcommands

Pass the subcommand as the skill argument.

- **`resolve <file>`** — print the kind(s)
  assigned to `<file>` and the merged rule
  config. Required when the user asks about a
  specific file.
- **`list`** — enumerate all declared kinds in
  the workspace config.

When no argument is given, default to `list`.

## Steps

1. Locate the workspace root:

   ```bash
   git rev-parse --show-toplevel
   ```

   Run every subsequent command from that path.
   Without `git`, use the current directory.

2. Detect the mdsmith CLI. Try, in order:

   ```bash
   mdsmith version
   ```

   ```bash
   go run ./cmd/mdsmith version
   ```

   If only the second succeeds, substitute
   `go run ./cmd/mdsmith` for `mdsmith` below.

3. Run the subcommand:

   For `resolve <file>`:

   ```bash
   mdsmith kinds resolve -- <file>
   ```

   For `list`:

   ```bash
   mdsmith kinds list
   ```

4. Surface the full stdout. If the command exits
   non-zero, surface stderr as well.
