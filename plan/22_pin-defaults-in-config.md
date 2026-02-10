# Pin Rule Defaults in .tidymark.yml

## Goal

Update the project's own `.tidymark.yml` to explicitly list all rule default
settings (via `tidymark init` output), so that upstream default changes in
future versions don't silently alter linting behavior.

## Prerequisites

- Plan 14 (config settings) — `DumpDefaults()` and `Configurable`
  already implemented
- Plan 15 (CLI subcommands) — `tidymark init` already implemented

## Tasks

1. Run `tidymark init` in a temp directory to capture the full default config
   with all rule settings populated.

2. Merge the generated defaults into the existing `.tidymark.yml`, preserving
   the current `front-matter`, `ignore`, and `overrides` sections. The `rules:`
   block should list every rule with its default settings explicitly (e.g.
   `line-length: { max: 80, exclude: [code-blocks, tables, urls] }`).

3. Verify `tidymark check .` still exits 0 with the updated config.

4. Add a note in `README.md` (in the "Bootstrapping with `tidymark init`"
   section, if it exists) recommending that users commit the full defaults
   so upstream changes are explicit.

## Acceptance Criteria

- [ ] `.tidymark.yml` contains explicit default settings for all
  configurable rules
- [ ] Non-configurable rules are listed as `true` (enabled)
- [ ] Existing `ignore` and `overrides` sections are preserved
- [ ] `tidymark check .` exits 0
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
