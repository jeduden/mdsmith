---
id: 83
title: 'Security hardening batch'
status: "🔲"
summary: >-
  Bundle of low-risk security fixes: ANSI sanitization,
  path traversal boundary, atomic writes, schema fs.FS,
  catalog injection warning, CUE timeout, include size
  limit.
---
# Security hardening batch

## Goal

Ship seven small, low-risk security improvements as a
single batch. Each fix is independent but too small
to warrant its own plan.

## Fixes

### A. ANSI escape sanitization (~15 LOC)

Strip C0/C1 control characters from user-controlled
strings in `TextFormatter` output to prevent terminal
injection.

Add to `internal/output/text.go`:

```go
func sanitizeTerminal(s string) string {
    return strings.Map(func(r rune) rune {
        if r == '\t' || r == '\n' || r == '\r' {
            return r
        }
        if r < 0x20 || r == 0x7f ||
            (r >= 0x80 && r <= 0x9f) {
            return -1
        }
        return r
    }, s)
}
```

Apply to `d.File`, `d.Message` in `Format`, and
`line` in `writeSourceLine`. Do not modify
`JSONFormatter` (Go's `encoding/json` already escapes
control chars).

### B. Path traversal boundary (~10 LOC + plumbing)

Add project-root boundary check to
cross-file-reference-integrity. Add `RootDir string`
to `lint.File`, populate from `engine.Runner.RootDir`.
In `resolveTargetFile`, before `os.Stat`:

```go
rel, err := filepath.Rel(absRoot, absResolved)
if err != nil ||
    rel == ".." ||
    strings.HasPrefix(
        rel,
        ".."+string(filepath.Separator),
    ) {
    return targetFile{}, false
}
```

### C. Atomic write in fix mode (~20 LOC)

Replace `os.WriteFile` in `fix.go:fixFile` with
temp-file-then-rename:

```go
tmp, err := os.CreateTemp(filepath.Dir(path), ".mdsmith-fix-*")
if err != nil {
    return err
}
tmpPath := tmp.Name()
defer func() {
    if tmpPath != "" {
        _ = os.Remove(tmpPath)
    }
}()
if _, err := tmp.Write(out); err != nil {
    tmp.Close()
    return err
}
if err := tmp.Close(); err != nil {
    return err
}
if err := os.Chmod(tmpPath, info.Mode()); err != nil {
    return err
}
if err := os.Rename(tmpPath, path); err != nil {
    return err
}
tmpPath = ""
```

### D. Schema read via fs.FS (~5 LOC)

Replace `os.ReadFile(r.Schema)` in
`requiredstructure/rule.go:82` with
`fs.ReadFile(f.RootFS, r.Schema)`. Fall back to
`f.FS` when `RootFS` is nil.

### E. Catalog injection lint warning (~20 LOC)

In the catalog rule's template rendering path, emit
a diagnostic when an interpolated front-matter value
contains embedded newlines or unbalanced `](`
sequences. Diagnostic only — no escaping.

### F. CUE evaluation resource limit (~5 LOC)

The CUE API (`CompileString`, `Unify`, `Validate`) is
not context-aware, so `context.WithTimeout` cannot
interrupt long-running evaluation. Rely on
`GOMEMLIMIT` at process level to cap memory. If a
true wall-clock limit is needed in the future, run
CUE evaluation in a separate OS process that can be
killed on timeout.

### G. Include/catalog size limit (depends on plan 81)

Replace `fs.ReadFile` with `ReadFSFileLimited` at
`include/rule.go:194`, `catalog/rule.go:395,468`.
Thread `MaxInputBytes` via `lint.File`.

## Tasks

1. Add `sanitizeTerminal` to `output/text.go`; apply
   to `d.File`, `d.Message`, and source lines
2. Add `RootDir` to `lint.File`; populate in runner
   and fixer
3. Add `filepath.Rel` boundary check in
   `resolveTargetFile`
4. Replace `os.WriteFile` with temp-file-then-rename
   in `fix.go`
5. Replace `os.ReadFile(r.Schema)` with
   `fs.ReadFile(f.RootFS, r.Schema)`
6. Add newline and `](` detection in catalog template
   rendering; emit diagnostic
7. Set `GOMEMLIMIT` at process startup to cap memory
   for CUE and other unbounded operations
8. Replace `fs.ReadFile` with `ReadFSFileLimited` at
   include and catalog read sites (after plan 81)
9. Add tests for each fix (A through G)

## Acceptance Criteria

- [ ] ANSI escape bytes (0x1B, 0x9B, 0x07) stripped
      from text output; tab/newline preserved
- [ ] Links traversing above `RootDir` are silently
      skipped; links within root work
- [ ] `mdsmith fix` uses atomic temp-file-then-rename
- [ ] Schema read via `f.RootFS`; absolute/`../` paths
      rejected
- [ ] Catalog values with `\n` or `](` produce a
      diagnostic
- [ ] `GOMEMLIMIT` set at process startup to bound
      memory for CUE and other operations
- [ ] Include/catalog reads bounded by
      `MaxInputBytes` (after plan 81)
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
