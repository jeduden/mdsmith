# Remove Legacy CLI Backwards Compatibility

## Goal

Remove the deprecated flat-flag CLI invocation style
(`tidymark [flags] [files...]`) so that subcommands (`check`, `fix`,
`init`) are the only interface, simplifying the CLI code and
eliminating the deprecation warning path.

## Prerequisites

- Plan 15 (CLI subcommands) — the subcommand interface must be in place

## Tasks

1. Remove `looksLikeLegacyInvocation()` and `runLegacy()` from
   `cmd/tidymark/main.go`. Update the top-level dispatch in `run()` so
   that unrecognized first arguments (file paths, old flags like `--fix`,
   `--no-color`) produce an "unknown command" error with usage, exit 2.

2. Remove the backwards-compatibility E2E test from
   `cmd/tidymark/e2e_test.go` (the test that passes `--no-color file.md`
   without a subcommand and expects a deprecation warning).

3. Update `README.md` usage examples: ensure all examples use the
   subcommand form (`tidymark check`, `tidymark fix`). Remove any
   mentions of the deprecated flat-flag style.

4. Update `CLAUDE.md` CLI usage section to reflect subcommand-only
   interface.

5. Update the lefthook pre-commit example in `README.md` to use
   `tidymark check {staged_files}` and `tidymark fix {staged_files}`.

6. Update `.github/workflows/ci.yml` tidymark job if it uses the old
   invocation style — change to `./tidymark check .`.

## Acceptance Criteria

- [ ] `looksLikeLegacyInvocation` and `runLegacy` are removed
- [ ] `tidymark file.md` (no subcommand) exits 2 with unknown command error
- [ ] `tidymark --fix file.md` exits 2 with unknown command error
- [ ] All usage examples in README.md and CLAUDE.md use subcommand form
- [ ] CI workflow uses `tidymark check .`
- [ ] E2E tests updated (legacy test removed)
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
