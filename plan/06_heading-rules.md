# 06: Heading Rules (TM002-TM005, TM013, TM017-TM018)

## Goal

Implement seven heading-related rules using the goldmark AST (`ast.Heading`
and related nodes).

## Rules

| Rule | Name | Fixable |
|------|------|---------|
| TM002 | heading-style | yes |
| TM003 | heading-increment | no |
| TM004 | first-line-heading | no |
| TM005 | no-duplicate-headings | no |
| TM013 | blank-line-around-headings | yes |
| TM017 | no-trailing-punctuation-in-heading | no |
| TM018 | no-emphasis-as-heading | no |

## Tasks

1. Implement each rule in its own package under `internal/rules/`
2. Each rule walks the goldmark AST for heading nodes
3. Create testdata fixtures for each rule
4. Write behavioral tests

## Acceptance Criteria

### TM002: heading-style

- [ ] An ATX heading (`# Heading`) reports nothing when `style: atx` (default)
- [ ] A setext heading (`Heading\n=======`) reports a diagnostic when `style: atx`
- [ ] A setext heading reports nothing when `style: setext`
- [ ] An ATX heading reports a diagnostic when `style: setext`
- [ ] Fix converts setext headings to ATX (and vice versa) preserving heading text
- [ ] Headings inside code blocks are not flagged

### TM003: heading-increment

- [ ] `# H1` followed by `### H3` reports a diagnostic on the `### H3` line
- [ ] `# H1` → `## H2` → `### H3` reports nothing
- [ ] `## H2` as the first heading (skipping H1) reports a diagnostic
- [ ] Multiple violations in one file are all reported
- [ ] Headings inside code blocks are not flagged

### TM004: first-line-heading

- [ ] A file starting with `# Heading` reports nothing (default `level: 1`)
- [ ] A file starting with plain text reports a diagnostic on line 1
- [ ] A file starting with `## Heading` reports a diagnostic when `level: 1`
- [ ] A file starting with `## Heading` reports nothing when `level: 2`
- [ ] An empty file reports a diagnostic
- [ ] A file starting with a blank line followed by `# Heading` reports a
      diagnostic (first *line* must be the heading)

### TM005: no-duplicate-headings

- [ ] Two headings with the same text report a diagnostic on the second one
- [ ] Headings with different text report nothing
- [ ] Headings at different levels with the same text still report a diagnostic
- [ ] Headings inside code blocks are not counted

### TM013: blank-line-around-headings

- [ ] A heading preceded by non-blank text reports a diagnostic
- [ ] A heading followed by non-blank text reports a diagnostic
- [ ] A heading with blank lines before and after reports nothing
- [ ] A heading on line 1 (no line before) does not require a blank line before
- [ ] A heading on the last line (no line after) does not require a blank line after
- [ ] Fix inserts blank lines before/after headings as needed

### TM017: no-trailing-punctuation-in-heading

- [ ] `# Introduction.` reports a diagnostic (trailing `.`)
- [ ] `## Overview:` reports a diagnostic (trailing `:`)
- [ ] Trailing `,`, `;`, `!` each report a diagnostic
- [ ] `# Introduction` (no punctuation) reports nothing
- [ ] `# What is this?` — question mark is NOT flagged (only `.,:;!`)
- [ ] Heading text inside code blocks is not flagged

### TM018: no-emphasis-as-heading

- [ ] A standalone line `**Bold text**` (paragraph with only strong emphasis)
      reports a diagnostic
- [ ] A standalone line `*Italic text*` reports a diagnostic
- [ ] Bold/emphasis inline within a paragraph (e.g., `Some **bold** text`)
      does NOT report a diagnostic
- [ ] Emphasis inside code blocks is not flagged

### General

- [ ] Each rule is enabled by default
- [ ] Each rule can be disabled via config
- [ ] Configurable settings (`style`, `level`) are read from config and applied
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
