# 03: Lint Engine

## Goal

Wire config + rules + file reading into a working pipeline. The binary can
accept file arguments and run rules against them.

## Tasks

1. File resolution (`internal/lint/files.go`)
   - Accept positional args: file paths, directories (recurse `*.md`/`*.markdown`),
     glob patterns
   - Filter against `ignore` patterns from config
   - No args + no stdin → exit 0 (graceful empty invocation)
2. Runner (`internal/lint/runner.go`)
   - For each file: read content, build `File` (parse AST once), determine
     effective rule config (base + overrides), run enabled rules, collect diagnostics
   - Sort diagnostics by file, line, column
3. Exit codes: 0 = clean, 1 = issues found, 2 = runtime error

## Acceptance Criteria

- [ ] Given a single `.md` file path, the runner reads it and runs all enabled rules
- [ ] Given a directory path, the runner recursively finds all `*.md` and
      `*.markdown` files
- [ ] Given a glob pattern (e.g., `docs/*.md`), the runner resolves matching files
- [ ] Files matching `ignore` patterns are excluded from linting
- [ ] No arguments and no stdin → exits 0 with no output
- [ ] A nonexistent file path → exits 2 with an error message on stderr
- [ ] A non-readable file → exits 2 with an error message on stderr
- [ ] With a mock rule that always reports a violation: the runner returns
      diagnostics and the binary exits 1
- [ ] With a mock rule that reports nothing: the binary exits 0
- [ ] Diagnostics are sorted by file path, then line, then column
- [ ] Per-file overrides apply: a rule disabled for `CHANGELOG.md` does not
      produce diagnostics for that file
- [ ] Multiple files are linted in a single invocation and diagnostics from
      all files are collected
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
