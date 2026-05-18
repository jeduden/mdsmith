---
name: mdsmith-check
description: >-
  Run `mdsmith check` on Markdown files in the
  current workspace and surface lint diagnostics.
  Useful without the `mdsmith-lsp` plugin — gives
  the same findings in a text report. Trigger when
  the user asks to "check my markdown", "run
  mdsmith check", "lint these files", or "show
  lint errors".
user-invocable: true
argument-hint: "[path | .]"
allowed-tools: "Bash(mdsmith:*)"
---
# mdsmith check

Run `mdsmith check` on the argument, defaulting
to `.` (entire workspace) when no argument is
given.

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

3. Run the check:

   ```bash
   mdsmith check -- <path>
   ```

   where `<path>` is the skill argument, or `.`
   when none was given.

4. Surface the full stdout. If the command exits
   non-zero, also show stderr and end with a
   summary line: `N diagnostic(s) found`.

## Notes

`mdsmith check` exits 0 when all files pass,
1 when diagnostics are found, and 2 on a fatal
error (bad config, binary missing, etc.). Only
exit code 2 means the CLI itself failed.
