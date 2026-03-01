---
id: 41
title: Verbose Mode
status: ✅
---
# Verbose Mode

## Goal

Add a `--verbose` / `-v` flag to the `check` and `fix`
commands so users can see which files are being processed,
which config is loaded, and which rules run on each file.

## Tasks

1. Add `--verbose` / `-v` boolean flag (default `false`) to
   the shared subcommand flags in `cmd/mdsmith/main.go`.

2. Create `internal/log/logger.go` with a minimal verbose
   logger:

   ```go
   type Logger struct{ Enabled bool }
   func (l *Logger) Printf(format string, args ...any)
   ```

   Writes to stderr when `Enabled` is true, no-op otherwise.

3. Pass the logger into `engine.Runner` and `fix.Fixer`.
   Log at these points:

  - Config discovered: `config: .mdsmith.yml`
  - File resolved: `file: README.md`
  - Rule applied: `rule: MDS001 line-length (max: 80)`
  - Fix pass: `fix: pass 1 on README.md`
  - Fix stable: `fix: README.md stable after 2 passes`

4. When `--quiet` and `--verbose` are both set, `--quiet`
   wins (verbose is suppressed).

5. Add verbose output to the text formatter summary line.
   When verbose, print a trailing summary:

   ```text
   checked 12 files, 3 issues found
   ```

6. Update CLI help text and CLAUDE.md flags table.

7. E2E tests:

  - `--verbose` shows config and file lines on stderr
  - `--quiet` suppresses verbose even when both set
  - Verbose output does not appear in `--format json`
     stdout

8. Unit tests for the logger (enabled/disabled).

## Acceptance Criteria

- [ ] `--verbose` / `-v` flag accepted by check and fix
- [ ] Config path logged to stderr when verbose
- [ ] Each processed file logged to stderr when verbose
- [ ] Rules applied per file logged when verbose
- [ ] `--quiet` suppresses verbose output
- [ ] Summary line printed when verbose
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
