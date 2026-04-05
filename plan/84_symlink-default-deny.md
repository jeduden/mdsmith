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
- Deprecate `--no-follow-symlinks` CLI flag: accept
  silently (it's now the default).

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

1. [x] Replace `NoFollowSymlinks []string` with
   `FollowSymlinks bool` in `config.Config`
2. [x] Replace `--no-follow-symlinks` with
   `--follow-symlinks` in CLI flag sets
3. [x] Update `ResolveOpts` to use `FollowSymlinks bool`
4. [x] Update `walkDir` to skip symlinks by default
5. [x] Update `resolveGlob` to skip symlinks by default
6. [x] Add deprecation warning for old config key
7. [x] Update tests in `files_test.go`
8. [x] Add integration test: symlink to file outside
   project is skipped by default, followed with
   `--follow-symlinks`

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
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
