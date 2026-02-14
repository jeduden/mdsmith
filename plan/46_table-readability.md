---
id: 46
title: Design table readability measure
status: "✅"
---
# Design table readability measure

## Goal

Design a way to check markdown table readability.
MDS023/MDS024 skip tables today. A new rule should flag
tables that are hard to read.

## Tasks

1. Research what makes markdown tables hard to read:
   column count, cell word count, total row count,
   column-width variance, and nesting depth
2. Survey existing linters (markdownlint, Vale, textlint)
   for table-specific checks
3. Propose candidate metrics with concrete thresholds
   (e.g. max columns, max words per cell, max rows)
4. Decide whether to extend MDS023/MDS024 or create a
   dedicated table rule (e.g. MDS025 `table-complexity`)
5. Write the rule spec README following `rules/proto.md`
6. Implement the rule with tests
7. Add good/bad fixtures under the rule directory
8. Verify `mdsmith check .` passes

## Decision

Create a dedicated table rule (`MDS026 table-readability`).
Do not extend paragraph rules (`MDS023` and `MDS024`).
Table complexity checks are different from paragraph checks.

## Acceptance Criteria

- [x] Decision documented: new rule vs. extension
- [x] Rule spec README exists with settings and examples
- [x] Implementation with unit tests
- [x] Good and bad fixtures pass `mdsmith check .`
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
