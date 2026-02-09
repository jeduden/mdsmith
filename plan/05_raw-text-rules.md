# 05: Raw-Text Rules (TM006-TM009)

## Goal

Implement the four rules that operate on raw source bytes/lines without
needing the Markdown AST.

## Rules

| Rule | Name | Fixable |
|------|------|---------|
| TM006 | no-trailing-spaces | yes |
| TM007 | no-hard-tabs | yes |
| TM008 | no-multiple-blanks | yes |
| TM009 | single-trailing-newline | yes |

## Tasks

1. Implement each rule in its own package under `internal/rules/`
2. Register each rule in the registry (via `init()` or explicit registration)
3. Create testdata fixtures for each rule (good + bad examples)
4. Write behavioral tests for Check and Fix

## Acceptance Criteria

### TM006: no-trailing-spaces

- [ ] A line ending with spaces reports a diagnostic at the correct line/column
- [ ] A line ending with tabs reports a diagnostic
- [ ] A line with no trailing whitespace reports nothing
- [ ] An empty file reports nothing
- [ ] Fix removes all trailing spaces/tabs from every line
- [ ] Fix preserves lines that have no trailing whitespace

### TM007: no-hard-tabs

- [ ] A line containing a tab character reports a diagnostic at the tab's column
- [ ] Multiple tabs on one line report one diagnostic per
      tab (or one per line — match spec)
- [ ] A file with no tabs reports nothing
- [ ] Fix replaces each tab with spaces

### TM008: no-multiple-blanks

- [ ] Two consecutive blank lines report a diagnostic on the second blank line
- [ ] Three consecutive blank lines report diagnostics
- [ ] A single blank line between paragraphs reports nothing
- [ ] A file with no blank lines reports nothing
- [ ] Fix collapses consecutive blank lines to a single blank line

### TM009: single-trailing-newline

- [ ] A file ending without `\n` reports a diagnostic
- [ ] A file ending with `\n\n` (multiple trailing newlines)
      reports a diagnostic
- [ ] A file ending with exactly one `\n` reports nothing
- [ ] An empty file (0 bytes) reports a diagnostic
- [ ] Fix adds `\n` to a file that lacks it
- [ ] Fix removes extra trailing newlines, leaving exactly one

### General

- [ ] Each rule is enabled by default
- [ ] Each rule can be disabled via config (`rule-name: false`)
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
