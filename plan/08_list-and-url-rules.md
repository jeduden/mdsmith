# 08: List & URL Rules (TM012, TM014, TM016)

## Goal

Implement three rules covering lists and bare URLs using the goldmark AST
(`ast.List`, `ast.ListItem`, `ast.Link`, `ast.AutoLink`, `ast.Text`).

## Rules

| Rule | Name | Fixable |
|------|------|---------|
| TM012 | no-bare-urls | yes |
| TM014 | blank-line-around-lists | yes |
| TM016 | list-indent | yes |

## Tasks

1. Implement each rule in its own package under `internal/rules/`
2. Each rule uses AST nodes for correctness
3. Create testdata fixtures for each rule
4. Write behavioral tests

## Acceptance Criteria

### TM012: no-bare-urls

- [ ] `Visit https://example.com for info` reports a diagnostic on the URL
- [ ] `Visit <https://example.com> for info` reports nothing (angle-bracket link)
- [ ] `Visit [example](https://example.com)` reports nothing (inline link)
- [ ] A URL inside a fenced code block is NOT flagged
- [ ] A URL inside an inline code span (`` `https://example.com` ``) is NOT flagged
- [ ] A URL inside a link destination `[text](url)` is NOT flagged
- [ ] A URL in a reference definition `[label]: url` is NOT flagged
- [ ] Multiple bare URLs on different lines are each reported
- [ ] Fix wraps bare URLs in angle brackets: `https://x.com` → `<https://x.com>`
- [ ] The diagnostic column points to the start of the URL

### TM014: blank-line-around-lists

- [ ] A list preceded by non-blank text reports a diagnostic
- [ ] A list followed by non-blank text reports a diagnostic
- [ ] A list with blank lines before and after reports nothing
- [ ] A list at the start of the file does not require a blank line before
- [ ] A list at the end of the file does not require a blank line after
- [ ] Nested lists do not trigger the rule at the inner list boundary
- [ ] A list immediately after a heading (no blank line) reports a diagnostic
- [ ] Fix inserts blank lines before/after lists as needed

### TM016: list-indent

- [ ] A nested list indented with 2 spaces reports nothing (default `spaces: 2`)
- [ ] A nested list indented with 4 spaces reports a diagnostic when `spaces: 2`
- [ ] A nested list indented with 4 spaces reports nothing when `spaces: 4`
- [ ] Deeply nested lists (3+ levels) are checked at each level
- [ ] Ordered lists (`1.`) are checked the same as unordered
- [ ] Fix adjusts indentation to match the configured `spaces` value
- [ ] List items inside code blocks are not flagged

### General

- [ ] Each rule is enabled by default
- [ ] Each rule can be disabled via config
- [ ] Configurable settings (`spaces`) are read from config and applied
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
