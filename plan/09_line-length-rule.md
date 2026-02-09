# 09: Line-Length Rule (TM001)

## Goal

Implement TM001 (line-length) using the AST for correct `strict: false`
behavior. This rule is placed after code block rules because it needs AST
knowledge of code block boundaries.

## Rules

| Rule | Name | Fixable |
|------|------|---------|
| TM001 | line-length | no |

## Tasks

1. Implement in `internal/rules/linelength/`
2. Use AST to determine which source lines are inside
   fenced/indented code blocks
3. Create testdata fixtures
4. Write behavioral tests

## Acceptance Criteria

### TM001: line-length

- [ ] A line exceeding 80 characters reports a diagnostic (default `max: 80`)
- [ ] A line with exactly 80 characters reports nothing
- [ ] A line with 81 characters reports a diagnostic with message including
      the actual length and the limit (e.g., `"line too long (81 > 80)"`)
- [ ] Setting `max: 120` changes the threshold; a 100-char line reports nothing
- [ ] `strict: false` (default): a line inside a fenced code block that exceeds
      the limit does NOT report a diagnostic
- [ ] `strict: false`: a line inside an indented code block (4-space) that
      exceeds the limit does NOT report a diagnostic
- [ ] `strict: false`: a line whose only non-whitespace
      content is a URL (e.g., `https://very-long-url...`)
      does NOT report a diagnostic
- [ ] `strict: true`: long lines inside code blocks ARE flagged
- [ ] `strict: true`: long URL-only lines ARE flagged
- [ ] The diagnostic column points to the character position where the limit
      is exceeded (column = max + 1)
- [ ] Multiple long lines in one file each produce a separate diagnostic
- [ ] An empty file reports nothing

### General

- [ ] The rule is enabled by default with `max: 80`, `strict: false`
- [ ] The rule can be disabled via config (`line-length: false`)
- [ ] Settings `max` and `strict` are read from config
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
