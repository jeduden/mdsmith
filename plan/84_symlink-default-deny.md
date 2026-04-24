---
id: 84
title: 'Symlink default-deny for file discovery'
status: "✅"
summary: >-
  Skip symlinks by default during directory walks;
  add --follow-symlinks opt-in flag.
---
# Symlink default-deny for file discovery

## Goal

Prevent symlink-based attacks. A malicious symlink in
a repo can cause `mdsmith check` to read or
`mdsmith fix` to overwrite files outside the project.

## Background

`filepath.Walk` in `walkDir` follows file symlinks by
default. The existing `--no-follow-symlinks` CLI flag
is opt-in and skips all symlinks by setting the
pattern-based `no-follow-symlinks` config key to `**`.
A symlink like
`ln -s /etc/cron.d/jobs evil.md` in a repo causes
`mdsmith fix .` to overwrite the target.

Among compared linters, only Prettier v3 rejects
symlinks (PR #14627). textlint is incidentally safe
(glob default). markdownlint, Vale, and remark-lint
all follow symlinks.

## Design

### Invert the default

Replace `NoFollowSymlinks []string` (pattern-based
opt-out) with `FollowSymlinks bool` (global opt-in).

In `walkDir`, skip all entries where
`info.Mode()&os.ModeSymlink != 0` unless
`FollowSymlinks` is true.

### CLI flag

Replace `--no-follow-symlinks` with
`--follow-symlinks`:

```bash
mdsmith check --follow-symlinks .
mdsmith fix --follow-symlinks .
```

### Config key

Replace `no-follow-symlinks` with `follow-symlinks`:

```yaml
follow-symlinks: true  # opt-in, default false
```

### Migration

- Deprecate `no-follow-symlinks` config key: if
  present, emit a warning suggesting migration to
  the new `follow-symlinks: false` default.
- Remove `--no-follow-symlinks` CLI flag outright:
  the polarity flipped (`--follow-symlinks` is the
  new opt-in) and keeping a negated sibling is both
  redundant with the new secure default and confusing
  next to it. Passing the removed flag errors out on
  parse (exit 2), same as any other unknown flag.

### Write-side protection

The write-side TOCTOU is handled by plan 83 section C
(atomic writes). `os.Rename(tmp, path)` replaces the
symlink
itself, not the target — no separate Lstat check
needed.

### WalkDir migration (optional)

Consider migrating from `filepath.Walk` to
`filepath.WalkDir` (Go 1.16+). `WalkDir` provides
`d.Type()` with `fs.ModeSymlink` without extra
`os.Lstat` calls. This makes symlink detection cheaper.

## Tasks

1. [x] Replaced `NoFollowSymlinks []string` with
   `FollowSymlinks bool` in `config.Config`; kept
   `LegacyNoFollowSymlinks` for deprecation parsing
2. [x] Replaced `--no-follow-symlinks` with
   `--follow-symlinks`; old flag removed outright
3. [x] Updated `ResolveOpts` to use `FollowSymlinks bool`
4. [x] Updated `walkDir` to skip symlinks by default
5. [x] Updated `resolveGlob` to skip symlinks by default
6. [x] Added deprecation warning for old config key
   (emitted by `cmd/mdsmith.loadConfig` once per run)
7. [x] Updated `files_test.go` and `lint_coverage_test.go`
8. [x] Added integration tests in
   `cmd/mdsmith/e2e_symlink_default_deny_test.go`:
   external-target symlink skipped by default,
   followed with `--follow-symlinks`, config-key
   opt-in, legacy-config deprecation warning, and
   fix TOCTOU behavior (symlink replaced, target
   untouched)

## Acceptance Criteria

- [x] Symlinks are skipped by default in directory
      walks
- [x] `--follow-symlinks` flag enables symlink
      following
- [x] `follow-symlinks: true` in config enables
      symlink following
- [x] Old `no-follow-symlinks` config emits
      deprecation warning
- [x] Both `check` and `fix` respect the setting
- [x] All tests pass: `go test ./...` (except
      pre-existing `internal/corpus` failures
      tracked by plan 90)
- [x] `go tool golangci-lint run` reports no issues
