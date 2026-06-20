---
id: 2606192025
title: "Replace os.DirFS with os.OpenRoot to contain symlink escapes"
status: "🔳"
summary: >-
  os.DirFS follows symlinks outside the workspace root in Go 1.25.
  Replace with os.OpenRoot (RESOLVE_BENEATH) at all RootFS construction
  sites so include and catalog cannot read files outside the project root
  via within-workspace symlinks. Closes S001 and S002 (HIGH) from the
  2026-06-19 full-repo audit.
model: sonnet
---
# Replace os.DirFS with os.OpenRoot to contain symlink escapes

## Goal

Close S001 and S002 (HIGH) from the [2026-06-19 full-repo security
audit](../docs/security/2026-06-19-full-repo-audit/report.md).

`os.DirFS` follows symlinks in Go 1.25. A within-workspace symlink
whose target lies outside the project root passes the dot-dot check in
`resolveIncludePath` and is opened by `os.DirFS`. The same flaw lets
`doublestar.GlobWalk` in the catalog rule read external files. Replacing
`os.DirFS` with `os.OpenRoot` enforces `RESOLVE_BENEATH` at the OS
level.

**S001 (high, CWE-73).** `resolveIncludePath` opens the resolved path
through `f.RootFS = os.DirFS(rootDir)` (`internal/rules/include/
rule.go:253-267`). A symlink `docs/secret.md -> /etc/passwd` inside
the workspace passes the dot-dot check and is read by `os.DirFS`.
During `mdsmith fix` the content is embedded in the generated section.
During `mdsmith check` it is read for comparison.

**S002 (high, CWE-73).** The catalog rule at
`internal/rules/catalog/rule.go:893-906` walks glob matches via
`res.fs = os.DirFS(rootDir)`. Symlink entries are passed to
`fs.Stat`, which follows them to their target. The rule then reads
front matter from that target file.

## Tasks

1. Write a failing e2e test in `internal/integration/` (or a new
   `testdata/symlink_escape/` subtree) that:

  - creates a temp workspace with `docs/secret.md -> /etc/passwd`
    (symlink) and a Markdown file with
    `<?include file: docs/secret.md ?>...<?/include?>`,
  - runs the include rule's `Fix` and asserts it returns an error or
    empty body (not `/etc/passwd` content).

2. Write a second failing e2e test for the catalog variant:

  - workspace with `docs/leaked.md -> /etc/hostname` (symlink) and
    `<?catalog glob: docs/*.md ?>`,
  - assert the catalog emits no rows (not the hostname contents).

3. In `pkg/mdsmith/workspace.go:100-106`, change `OSWorkspace.FS()` to
   open an `*os.Root` via `os.OpenRoot(root)` and return `root.FS()`
   instead of `os.DirFS(root)`. Propagate the error — `FS()` must
   return `(fs.FS, error)` or the caller must handle it. Adjust the
   `Workspace` interface accordingly.
4. Update `internal/lint/file.go:286` where `f.RootFS = os.DirFS(dir)`
   is set — switch to `os.OpenRoot(dir).FS()`.
5. Confirm both e2e tests now pass.
6. Add a unit test in `pkg/mdsmith/workspace_test.go` asserting
   `OSWorkspace.FS()` on a root with an escaping symlink returns an
   error on `Open`.
7. Run `go test ./...` and `go tool golangci-lint run`.
8. Run `go run ./cmd/mdsmith check .` to confirm doc linting still passes.

## Acceptance Criteria

- [ ] `<?include file: symlink-to-outside ?>` on a within-workspace
  symlink to a path outside the project root is refused (error
  diagnostic, no content embedded).
- [ ] `<?catalog glob: *.md ?>` on a pattern matching a within-workspace
  symlink to an outside file emits no catalog row for that symlink.
- [ ] `OSWorkspace.FS()` wraps `os.OpenRoot`; `os.DirFS` is no longer
  called in the `RootFS` construction path.
- [ ] Within-workspace symlinks to files *inside* the root continue to
  work (positive test).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
