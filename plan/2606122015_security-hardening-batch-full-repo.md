---
id: 2606122015
title: "Security hardening batch — 2026-06-12 full-repo audit"
status: "🔲"
summary: >-
  Low/informational hardening from the 2026-06-12 full-repo audit:
  make the MDS040 gate non-bypassable by attacker-controlled config
  (S002), fix rename symlink write-through when follow-symlinks is
  enabled (S007), and add a warning when fix encounters recipes in an
  unfamiliar repo.
model: sonnet
---
# Security hardening batch — 2026-06-12 full-repo audit

## Goal

Close findings S002 and S007 from the [2026-06-12 full-repo audit
report](../docs/security/2026-06-12-full-repo-audit/report.md).

**S002 (low, confirmed).** `checkMDS040Gate` (`buildpass.go:50-84`)
returns true (gate open) when `recipe-safety` is absent or disabled:
`if !ok || !rc.Enabled { return true }`. An attacker-controlled
`.mdsmith.yml` can set `recipe-safety: false`, bypassing the
shell-operator/interpreter checks and allowing recipe commands with
shell interpreters to execute via `mdsmith fix`. The gate is the sole
pre-execution safety check.

**S007 (low, confirmed).** `writeFilePreservingMode`
(`rename.go:356-362`) uses `os.WriteFile`, which on POSIX follows
symlinks. When `--follow-symlinks` is enabled and a workspace file is
a symlink to an external file, `mdsmith rename` overwrites the
external file. The `fix` command uses a safe temp-file-then-rename
pattern (`atomicWriteFile`) that replaces the symlink itself — the
rename command is missing the same pattern.

## Tasks

### S002 — MDS040 gate hardening

- [ ] **Red**: write a test that disables `recipe-safety` in config,
  adds a recipe with `command: sh -c 'echo pwned'`, and asserts that
  `checkMDS040Gate` still rejects it (returns false / returns an
  error).
- [ ] **Green** (Option A): when `build.recipes` is non-empty,
  always run the shell-safety check regardless of the
  `recipe-safety` rule toggle. The rule can still be disabled for
  diagnostic reporting, but the gate should not be bypassable.
- [ ] Alternatively (Option B): emit a prominent warning when
  `mdsmith fix` is run with recipes present and
  `recipe-safety: false` is set in config, telling the user they are
  running recipes without safety checks.
- [ ] Run `go test ./cmd/mdsmith/... ./internal/...`; all pass.

### S007 — rename symlink write-through

- [ ] **Red**: write a test that enables `follow-symlinks`, creates a
  symlink in a temp workspace pointing to an external temp file, runs
  `writeFilePreservingMode` on the symlink path, and asserts the
  external file is NOT modified (or the function returns an error).
- [ ] **Green**: replace `os.WriteFile` in `writeFilePreservingMode`
  with the temp-file-then-rename pattern from `atomicWriteFile`
  (fix.go): create a temp file in `filepath.Dir(path)`, write to it,
  then call `os.Rename(tmp, path)`. On POSIX, `os.Rename` replaces
  the symlink itself rather than following it.
- [ ] Run `go test ./cmd/mdsmith/...`; all pass.

## Acceptance Criteria

- With `recipe-safety: false` in config, `mdsmith fix` either rejects
  shell-interpreter recipes or displays a clear user-facing warning
  before executing any recipe.
- `mdsmith rename` with `--follow-symlinks` does not overwrite files
  outside the workspace via symlinks.
- All existing tests pass.
