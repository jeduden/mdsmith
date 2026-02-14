---
id: 47
title: Token Budget Awareness
status: ✅
---
# Token Budget Awareness

## Goal

Warn when Markdown files exceed a configurable token budget,
providing a more useful signal than line count for LLM context usage.

## Tasks

1. Define token counting modes: heuristic ratio vs tokenizer-based
   (model-specific), and choose a default.
2. Evaluate tokenizer integration options and asset handling
   (no network fetch at runtime).
3. Add rule configuration to select mode, ratio,
   tokenizer/encoding, and per-glob budgets.
4. Implement rule to report when tokens exceed the budget,
   including count and mode in output.
5. Update rule docs with performance/accuracy trade-offs
   and examples for both modes.

## Acceptance Criteria

- [ ] Rule supports both heuristic estimation and tokenizer-based counting.
- [ ] Rule warns when tokens exceed a configurable budget, per file or glob.
- [ ] Output includes token count, budget, and counting mode.
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
