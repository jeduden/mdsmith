---
id: 47
title: Token Budget Awareness
status: 🔲
template:
  allow-extra-sections: true
---
# Token Budget Awareness

## Goal

Warn when Markdown files exceed a configurable token budget, providing a more useful signal than line count for LLM context usage.

## Tasks

1. Define a token estimation strategy (e.g., word count * ratio) with configurable ratio and budget.
2. Add rule configuration and CLI surface to enable token budget checks per file or glob.
3. Implement rule to report when estimated tokens exceed the budget, including estimated count in output.
4. Update rule docs and examples with configuration guidance.

## Acceptance Criteria

- [ ] Rule estimates token count using a configurable ratio and word count.
- [ ] Rule warns when estimated tokens exceed a configurable budget.
- [ ] Output includes estimated tokens and the configured budget.
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
