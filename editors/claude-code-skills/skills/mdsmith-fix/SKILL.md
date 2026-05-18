---
name: mdsmith-fix
description: >-
  Run `mdsmith fix` on Markdown files in the
  current workspace. Auto-fixes whitespace,
  headings, code fences, bare URLs, list
  indentation, table alignment, and generated
  sections (catalog, include, toc, build) in
  place, looping until edits stabilize. Trigger
  when the user asks to "fix my markdown",
  "run mdsmith fix", "clean up lint", or
  "regenerate sections".
user-invocable: true
argument-hint: "[path | .]"
allowed-tools: "Bash(mdsmith:*)"
---
# mdsmith fix

Run `mdsmith fix` on the argument, defaulting
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

3. Run the fix:

   ```bash
   mdsmith fix -- <path>
   ```

   where `<path>` is the skill argument, or `.`
   when none was given.

4. If the command exits non-zero, surface the
   full stderr to the user.

5. Report how many files were rewritten (parse
   the last line of stdout, which reads
   `N file(s) fixed`).

## Notes

`mdsmith fix` loops up to 10 passes. It exits
0 even when files change — a non-zero exit
means the CLI itself failed (binary missing,
config parse error, etc.).
