---
id: 43
title: Default File Discovery
status: ✅
---
# Default File Discovery

## Goal

Let `mdsmith check` and `mdsmith fix` find files from the
config when no file arguments are given, so users can run
the tool without listing files every time.

## Tasks

1. Add a `files` key to the config schema. It holds a list
   of glob patterns. The default value is
   `["**/*.md", "**/*.markdown"]`.

2. Build file discovery logic in `internal/discovery/`. When
   no file arguments are given and stdin is not piped, read
   the `files` globs from the loaded config. Expand each glob
   and collect the results. Apply `.gitignore` filtering
   unless `--no-gitignore` is set.

3. Change stdin reading to require an explicit `-` argument.
   Remove the implicit `isStdinPipe` detection. When the user
   passes `-` as a file argument, read from stdin. Otherwise
   ignore stdin entirely.

4. Update the `check` command. When no file args and no `-`
   argument, call the discovery logic from step 2. If the
   config has no `files` key and no default applies, exit 0.

5. Update the `fix` command the same way. When no file args,
   use config globs to find files to fix.

6. If no `files` key exists in the config and no built-in
   default applies, exit 0 with no output. This keeps the
   graceful empty invocation for edge cases.

7. Add unit tests for the discovery package:

  - Config with `files: ["**/*.md"]` finds `.md` files
  - Empty `files` list returns no files
  - `.gitignore` patterns are respected
  - `--no-gitignore` disables filtering

8. Add integration tests for the CLI:

  - `mdsmith check` with no args uses config globs
  - `mdsmith check -` reads from stdin
  - `mdsmith fix` with no args uses config globs
  - Missing `files` key exits 0

9. Update `CLAUDE.md` and CLI help text. Document the new
   `files` config key and the `-` stdin argument.

## Acceptance Criteria

- [ ] Config schema accepts a `files` key with glob patterns
- [ ] Default `files` value is `["**/*.md", "**/*.markdown"]`
- [ ] `check` without args discovers files from config
- [ ] `fix` without args discovers files from config
- [ ] Passing `-` as a file arg reads from stdin
- [ ] Implicit stdin detection is removed
- [ ] `.gitignore` filtering applies to discovered files
- [ ] `--no-gitignore` disables filtering on discovered files
- [ ] No `files` key and no default exits 0
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
