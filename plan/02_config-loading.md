# 02: Config Loading

## Goal

Parse `.tidymark.yml`, merge defaults, support overrides and `--config` flag.

## Tasks

1. Config struct (`internal/config/config.go`):
   `Config` with `Rules map[string]RuleCfg`,
   `Ignore []string`, `Overrides []Override`.
   `RuleCfg` handles YAML union: `bool` or
   `map[string]any` via custom `UnmarshalYAML`.
2. Discovery — walk up from cwd to repo root (`.git`)
   or filesystem root looking for `.tidymark.yml`.
3. `--config` flag overrides discovery.
4. Merge logic — defaults (all rules enabled) + config
   file + overrides matched by file glob. Later overrides
   take precedence.

## Acceptance Criteria

- [ ] A valid `.tidymark.yml` with `rules`, `ignore`, and `overrides` parses
      into the correct `Config` struct
- [ ] `RuleCfg` correctly handles `false` (disabled), `true` (enabled with
      defaults), and an object with settings (e.g., `{max: 120}`)
- [ ] Invalid YAML returns a descriptive error
- [ ] Discovery finds `.tidymark.yml` in the current directory
- [ ] Discovery finds `.tidymark.yml` in a parent directory when not present
      in the current directory
- [ ] Discovery stops at the git root (`.git` directory) and does not search
      above it
- [ ] Discovery returns no config (defaults apply) when no `.tidymark.yml`
      exists anywhere in the path
- [ ] `--config /path/to/custom.yml` loads that file instead of discovering
- [ ] `--config` with a nonexistent file returns exit code 2
      with an error message
- [ ] Merge: without a config file, all 18 rules are enabled with their defaults
- [ ] Merge: `line-length: false` disables TM001; other rules remain enabled
- [ ] Merge: `line-length: {max: 120}` enables TM001 with `max=120`
- [ ] Overrides: `files: ["CHANGELOG.md"]` with `no-duplicate-headings: false`
      disables TM005 only for `CHANGELOG.md`
- [ ] Overrides: later overrides take precedence over earlier ones for the
      same file pattern
- [ ] Glob patterns in `ignore` and `overrides.files` match correctly
      (e.g., `"vendor/**"` matches `vendor/foo/bar.md`)
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
