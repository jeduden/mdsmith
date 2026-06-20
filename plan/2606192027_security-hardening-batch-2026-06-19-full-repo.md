---
id: 2606192027
title: "Security hardening batch — 2026-06-19 full-repo audit (low/info)"
status: "✅"
summary: >-
  Four low/informational hardening items from the 2026-06-19 full-repo
  audit: catalog file-count cap (S004), hasSymlinkAncestor empty-boundary
  guard (S005), explicit URL scheme rejection in include (S006), and
  bytelimit on githooksync os.ReadFile calls (S007).
model: sonnet
---
# Security hardening batch — 2026-06-19 full-repo audit (low/info)

## Goal

Address the four low-severity and informational findings from the
[2026-06-19 full-repo security
audit](../docs/security/2026-06-19-full-repo-audit/report.md). None is
immediately exploitable with high impact, but each removes a gap in an
existing defense layer.

**S004 (low, CWE-400).** `resolveGlobMatchesFrom` in
`internal/rules/catalog/rule.go:888-920` accumulates matched paths with
no cap. On a very large workspace, a wildcard catalog can cause OOM.

**S005 (low, CWE-61).** `hasSymlinkAncestor` in
`internal/lint/files.go:244-246` skips the ancestor Lstat scan when
both `os.Getwd()` fails and there is no `.git` root. A symlinked
directory component in an explicit path argument is then undetected.

**S006 (info, CWE-918).** `validateIncludeDirective` at
`internal/rules/include/rule.go:130-170` blocks absolute paths via
`filepath.IsAbs`. It does not block URL schemes. The safety depends
on `os.DirFS` making no network calls — an incidental guard. An
explicit scheme check removes that dependency.

**S007 (info, CWE-400).** The githooksync rule uses `os.ReadFile`
with no size cap on four reads: hook files and `.gitattributes`
(`internal/rules/githooksync/rule.go:180, 249, 295, 386`). Every
other read in this codebase uses `bytelimit.ReadFileLimited`.

## Tasks

### S004 — catalog file-count cap

- [x] Add `const maxCatalogMatches = 10_000` near `maxIncludeDepth` in
  `internal/rules/catalog/rule.go`.
- [x] In `buildCatalogEntries`, return a lint diagnostic (not a
  hard error) when the match count exceeds the cap, analogous to the
  `maxIncludeDepth` diagnostic in the include rule.
- [x] Add a unit test asserting the cap diagnostic fires when matches
  exceed the limit and that the generated section is left unchanged.
- [x] Document the limit in
  [docs/features/self-maintaining-sections.md](
  ../docs/features/self-maintaining-sections.md).

### S005 — hasSymlinkAncestor empty-boundary guard

- [x] In `internal/lint/files.go:244-246`, when `ancestorStopBoundary`
  is empty (both `os.Getwd()` failed and no `.git` ancestor), return an
  error rather than silently skipping the scan.
- [x] Add a unit test for the empty-boundary branch that confirms an
  error is returned.

### S006 — explicit URL scheme rejection in include validation

- [x] In `validateIncludeDirective`
  (`internal/rules/include/rule.go:130-170`), add a check for the
  `http://`, `https://`, and `file://` scheme prefixes alongside the
  existing `filepath.IsAbs` check. Return a diagnostic when matched.
- [x] Add a unit test with `file: http://example.com/foo` asserting the
  validation error fires.

### S007 — bytelimit on githooksync os.ReadFile calls

- [x] Replace the four `os.ReadFile` calls in
  `internal/rules/githooksync/rule.go` (lines 180, 249, 295, 386) with
  `bytelimit.ReadFileLimited` using a 1 MB cap (matching the config file
  cap).
- [x] Return a lint diagnostic when the file exceeds the limit.
- [x] Add a unit test asserting the cap diagnostic fires on an
  oversized hook file.

## Acceptance Criteria

- [x] A catalog directive matching more than 10,000 files emits a
  diagnostic and does not continue reading.
- [x] `hasSymlinkAncestor` returns an error (not `false`) when
  `ancestorStopBoundary` is empty.
- [x] `<?include file: http://example.com/foo ?>` emits a validation
  error.
- [x] A 2 MB `.git/hooks/pre-merge-commit` emits a diagnostic from the
  githooksync rule instead of being read fully into memory.
- [x] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
