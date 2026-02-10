# Make Version a Subcommand

## Goal

Change `--version`/`-v` from a global flag to a `version` subcommand so all
top-level arguments are subcommands, giving the CLI a consistent interface.

## Prerequisites

- Plan 15 (CLI subcommands) — already implemented

## Tasks

1. In `cmd/tidymark/main.go`, add `"version"` as a subcommand in the dispatch
   switch. It calls `printVersion()` and returns 0.

2. Remove the `--version`/`-v` global flag handling from the `run()` function
   (the `case "--version", "-v":` block before subcommand dispatch).

3. Update `usageText` to list `version` as a subcommand and remove
   `-v, --version` from the global flags section.

4. Update E2E tests in `cmd/tidymark/e2e_test.go`:

  - Change `--version` and `-v` tests to use `tidymark version`
  - Remove any tests that rely on `--version` or `-v` as flags
  - Add a test for unknown flag `-v` if desired

5. Update `README.md`: replace `--version` flag references with
   `tidymark version` subcommand.

6. Update `CLAUDE.md` flags table: remove `--version`/`-v` row, add
   `version` to the subcommand list if documented.

## Acceptance Criteria

- [ ] `tidymark version` prints version and exits 0
- [ ] `tidymark --version` is no longer recognized
  (exits 2 or triggers legacy compat)
- [ ] `usageText` lists `version` as a subcommand
- [ ] E2E tests updated and passing
- [ ] README.md and CLAUDE.md updated
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
