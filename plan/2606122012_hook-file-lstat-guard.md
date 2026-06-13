---
id: 2606122012
title: "Add lstat guard to hook-file install, uninstall, and status"
status: "✅"
summary: >-
  S001 from the 2026-06-12 git/LSP audit: ensurePreMergeCommitHook,
  runPreMergeCommitUninstall, and runPreMergeCommitStatus all read,
  write, or remove the hook path without a prior lstat check.
  WriteGitattributes already uses the correct pattern. Apply it here.
model: sonnet
---
# Add lstat guard to hook-file install, uninstall, and status

## Goal

Close finding S001 from the [2026-06-12 git/LSP audit
report](../docs/security/2026-06-12-git-lsp-audit/report.md).

Three functions read, write, or remove the hook path without a
prior lstat check:

- `ensurePreMergeCommitHook` (`cmd/mdsmith/mergedriver.go:728`):
  `os.ReadFile` then `os.WriteFile` with no guard. A symlink at
  `.git/hooks/pre-merge-commit` causes both to follow it, writing
  to an arbitrary path outside the workspace.
- `runPreMergeCommitUninstall` (`cmd/mdsmith/premergecommit.go:137`):
  `os.ReadFile` then `os.Remove` with no guard. A symlink causes
  Remove to delete an arbitrary file.
- `runPreMergeCommitStatus` (`cmd/mdsmith/premergecommit.go:186`):
  `os.ReadFile` with no guard.

`WriteGitattributes` (`internal/githooks/githooks.go:703`) is the
reference: `lstatFile` → reject if not regular → atomic
temp-then-rename. The hook-file operations should match.

## Tasks

- [x] **Red**: write a failing test for `ensurePreMergeCommitHook` that
  places a symlink at `.git/hooks/pre-merge-commit` and asserts the
  function returns an error instead of following the link.
- [x] **Green**: add an `os.Lstat` guard in `ensurePreMergeCommitHook`
  before `os.ReadFile`. Replace `os.WriteFile` with an atomic
  temp-then-rename helper (`writeHookFile`) mirroring
  `writeGitattributesFile`.
- [x] **Red**: write a failing test for `runPreMergeCommitUninstall`
  that places a symlink and asserts `os.Remove` is not called.
- [x] **Green**: add lstat guards in `runPreMergeCommitUninstall`
  before `os.ReadFile` and before `os.Remove`; reject if not regular.
- [x] **Red/Green**: add lstat guard to the `runPreMergeCommitStatus`
  `os.ReadFile` call; reject symlinks before reading.
- [x] Run `go test ./cmd/mdsmith/... ./internal/...`; all pass.
- [x] Run `go run ./cmd/mdsmith check .`; no regressions.

## Acceptance Criteria

- [x] A symlink at `.git/hooks/pre-merge-commit` causes `merge-driver
  install`, `pre-merge-commit install`, `pre-merge-commit uninstall`,
  and `pre-merge-commit status` to each return a clear error instead
  of following the link.
- [x] The fix uses the same pattern as `WriteGitattributes`.
- [x] All existing tests pass.
