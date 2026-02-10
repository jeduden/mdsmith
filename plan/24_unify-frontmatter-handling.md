# Unify Frontmatter Handling

## Goal

Consolidate the three duplicated frontmatter stripping and diagnostic
line-offset adjustment sites (runner, fixer, stdin) into a single
design centered on `lint.File`, fix the latent stdin Configurable
settings bug, and add missing integration tests for stdin.

## Prerequisites

None — can start immediately.

## Tasks

### A. Extend `lint.File`

1. Add `FrontMatter []byte` and `LineOffset int` fields to the
   `File` struct in `internal/lint/file.go`.

2. Add `NewFileFromSource(path string, source []byte,
   stripFrontMatter bool) (*File, error)` constructor. When
   `stripFrontMatter` is true it calls `StripFrontMatter`,
   stores the prefix in `FrontMatter`, computes `LineOffset`
   via `CountLines`, and parses only the stripped content.
   `NewFile` remains unchanged for backward compatibility.

3. Add `AdjustDiagnostics(diags []Diagnostic)` method on
   `*File` that adds `LineOffset` to each diagnostic's `Line`.

4. Add `FullSource(body []byte) []byte` method on `*File`
   that prepends `FrontMatter` to body (used by the fixer to
   re-prepend frontmatter when writing back).

### B. Extract shared helpers

5. Extract `config.IsIgnored(patterns []string, path string)
   bool` into `internal/config/ignore.go`. Remove the
   duplicate `isIgnored` methods from `runner.go` and
   `fix.go`.

6. Extract `engine.CheckRules(f *lint.File, rules []rule.Rule,
   effective map[string]config.RuleCfg) ([]lint.Diagnostic,
   []error)` into `internal/engine/check.go`. This contains
   the clone + ApplySettings + Check loop and calls
   `f.AdjustDiagnostics` on the result. Used by runner,
   fixer (final lint pass), and RunSource.

### C. Add `Runner.RunSource`

7. Add `RunSource(path string, source []byte) *Result` method
   to `Runner` in `internal/engine/runner.go`. It creates a
   `File` via `NewFileFromSource`, calls `CheckRules`, and
   returns the result. This replaces the inline stdin logic
   in `main.go` and fixes the missing Configurable settings
   application.

### D. Simplify callers

8. Simplify `Runner.Run` in `internal/engine/runner.go`:
   replace inline frontmatter stripping, the per-rule
   clone/check loop, and diagnostic adjustment with calls to
   `NewFileFromSource`, `CheckRules`, and
   `config.IsIgnored`.

9. Simplify `Fixer.Fix` in `internal/fix/fix.go`: use
   `NewFileFromSource` (store `File` for `FrontMatter`),
   use `FullSource(current)` for write-back, use
   `CheckRules` for the final lint pass, and use
   `config.IsIgnored` instead of the local method.

10. Replace `checkStdin()` body in `cmd/tidymark/main.go`
    with `runner.RunSource("<stdin>", source)`. Remove the
    manual frontmatter/rule loop code.

### E. Tests

11. Add unit tests in `internal/lint/file_test.go`:

  - `NewFileFromSource` with frontmatter: verify
    `LineOffset`, `FrontMatter`, `Source` (stripped)
  - `NewFileFromSource` without frontmatter: verify
    `LineOffset == 0`, `FrontMatter == nil`
  - `AdjustDiagnostics` shifts line numbers
  - `FullSource` prepends frontmatter bytes

12. Add unit tests in `internal/config/ignore_test.go` for
    `IsIgnored`.

13. Add unit tests in `internal/engine/check_test.go` for
    `CheckRules`: verify settings are applied, diagnostics
    are line-adjusted, disabled rules are skipped.

14. Add unit test for `Runner.RunSource`: verify it returns
    correct diagnostics with frontmatter line offsets and
    applied Configurable settings.

15. Add E2E tests in `cmd/tidymark/e2e_test.go`:

  - Stdin with frontmatter: verify reported line numbers
    reflect original file (not stripped content)
  - Stdin with Configurable settings: pipe a file with
    101-char lines through stdin with config
    `line-length: { max: 120 }`, verify no TM001
    diagnostic (settings applied)
  - Stdin with Configurable settings (violation): pipe
    a file with 130-char lines, verify TM001 fires
    (settings applied but line still too long)

## Acceptance Criteria

- [ ] `NewFileFromSource` handles frontmatter stripping
- [ ] `AdjustDiagnostics` and `FullSource` methods exist
- [ ] `config.IsIgnored` replaces duplicate methods
- [ ] `engine.CheckRules` replaces duplicate rule loops
- [ ] `Runner.RunSource` replaces stdin inline logic
- [ ] Stdin applies Configurable settings (bug fixed)
- [ ] Stdin reports correct frontmatter-offset line numbers
- [ ] Runner, fixer, and stdin all use unified code paths
- [ ] Unit tests for File, IsIgnored, CheckRules, RunSource
- [ ] E2E tests for stdin frontmatter + Configurable
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
