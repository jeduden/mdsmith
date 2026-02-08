# 04: Output Formatters

## Goal

`--format text` and `--format json` produce correct output on stderr.

## Tasks

1. Text formatter (`internal/output/text.go`)
   - Pattern: `file:line:col rule message`
   - ANSI color support (detect tty, respect `--no-color`)
2. JSON formatter (`internal/output/json.go`)
   - Array of diagnostic objects matching the spec in CLAUDE.md
3. `--quiet` flag — suppress non-error output
4. All lint output goes to stderr

## Acceptance Criteria

- [ ] Text format: a diagnostic at file `README.md`, line 10, column 5,
      rule TM001, message `"line too long (120 > 80)"` produces:
      `README.md:10:5 TM001 line too long (120 > 80)`
- [ ] Text format: multiple diagnostics are printed one per line, sorted
- [ ] Text format with color enabled: rule ID and file path are colored
      (verify ANSI escape sequences are present)
- [ ] `--no-color` suppresses ANSI escape sequences even when output is a tty
- [ ] JSON format: output is a valid JSON array
- [ ] JSON format: each element has fields `file`, `line`, `column`, `rule`,
      `name`, `severity`, `message` with correct types and values
- [ ] JSON format: empty diagnostics produce `[]`
- [ ] `--format text` is the default when no `--format` flag is given
- [ ] `--format json` switches to JSON output
- [ ] `--format invalid` exits 2 with an error message
- [ ] `--quiet` suppresses all output when there are no lint issues
- [ ] `--quiet` still outputs diagnostics when issues are found
- [ ] All output is written to stderr, not stdout
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
