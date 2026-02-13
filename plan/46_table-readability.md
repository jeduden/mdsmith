---
id: 46
title: Design table readability measure
status: "\U0001F532"
---
# Design table readability measure

## Goal

Design a way to check markdown table readability.
TM023/TM024 skip tables today. A new rule should flag
tables that are hard to read.

## Tasks

1. Research what makes markdown tables hard to read:
   column count, cell word count, total row count,
   column-width variance, and nesting depth
2. Survey existing linters (markdownlint, Vale, textlint)
   for table-specific checks
3. Propose candidate metrics with concrete thresholds
   (e.g. max columns, max words per cell, max rows)
4. Decide whether to extend TM023/TM024 or create a
   dedicated table rule (e.g. TM025 `table-complexity`)
5. Write the rule spec README following `rules/proto.md`
6. Implement the rule with tests
7. Add good/bad fixtures under the rule directory
8. Verify `tidymark check .` passes

## Acceptance Criteria

- [ ] Decision documented: new rule vs. extension
- [ ] Rule spec README exists with settings and examples
- [ ] Implementation with unit tests
- [ ] Good and bad fixtures pass `tidymark check .`
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
