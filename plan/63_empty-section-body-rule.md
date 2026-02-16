---
id: 63
title: Empty Section Body Rule
status: ✅
---
# Empty Section Body Rule

## Goal

Add a rule that reports headings whose section body is effectively empty,
so docs do not ship with placeholder or structure-only sections.

## Prior Art Summary

- `markdownlint` core rules do not include a dedicated empty-section-body
  rule. Nearest behavior is structural heading checks such as MD043.
- `remark-lint` has dedicated ecosystem coverage via
  `remark-lint-no-empty-sections`.
- `textlint` is plugin-driven and ships no bundled rules; the community rule
  `textlint-rule-no-empty-section` addresses this specific behavior.
- Vale focuses on style-rule patterns; section-body emptiness needs a custom
  check or external style package.

## Implemented Semantics

- Rule ID/name: `MDS030` / `empty-section-body`
- Category/default: `heading`, enabled by default
- Scope: check heading levels `2..6` by default
  (`min-level` / `max-level` settings)
- Meaningful content: paragraph, list, table, code block, or non-comment HTML
- Ignorable content: blank lines, HTML comments, or nested headings alone
- Exemption: explicit marker comment
  `<!-- allow-empty-section -->`

## Tasks

1. Define section-body semantics:
   what counts as meaningful content vs ignorable content between a heading
   and the next heading of same or higher level.
2. Review prior-art implementations in other linters and tools
   (for example `markdownlint`, `remark-lint`, Vale, and textlint):
   detect available rules, behavior differences, and known edge cases to
   avoid reinventing weak heuristics.
3. Define scope and exemptions:
   heading levels to check, and any allowed exceptions
   (for example explicit template placeholders).
4. Decide rule shape:
   new rule ID/name, category, default enablement, and configuration fields.
5. Implement heading-section analysis in the rule package,
   including stable line mapping for diagnostics.
6. Add unit tests for positive and negative cases:
   empty sections, comment-only sections, code/list/table content,
   nested headings, and edge boundaries at EOF.
7. Add rule docs and fixtures under `internal/rules/MDS030-*`,
   including config examples and diagnostic messages.
8. Integrate and verify in CLI/integration tests where needed.

## Acceptance Criteria

- [x] Rule reports headings that have no meaningful body content before the
      next same/higher-level heading.
- [x] Rule does not report sections with meaningful list, table, code, or
      paragraph content.
- [x] Prior-art behavior from other linters is summarized and key trade-offs
      are reflected in rule semantics or documented deviations.
- [x] Scope and exemptions are documented with examples.
- [x] Diagnostics include heading text, location, and clear remediation
      guidance.
- [x] Unit tests and fixtures cover representative edge cases.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
