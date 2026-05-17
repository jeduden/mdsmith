---
id: 180
title: Descriptive link text rule
status: "🔲"
model: sonnet
depends-on: [172]
summary: >-
  New rule MDS063 (provisional) for markdownlint MD059
  descriptive-link-text — flag non-descriptive link text
  like "click here", "here", "link", "more". Opt-in,
  configurable phrase list, no autofix.
---
# Descriptive link text rule

## Goal

Flag links whose visible text says nothing about the
destination ("click here", "read more"). Such text fails
screen readers and link-list navigation. This closes the
MD059 gap from the
[linter comparison](../docs/background/markdown-linters.md).

## Background

markdownlint MD059 ships a default banned list: `click
here`, `here`, `link`, `more`. It compares the trimmed,
lowercased link text. mdsmith reuses the same default and
the same comparison so migrating users get parity.

MD054 link-image-style is a separate concern. It is
already owned by
[plan 172](172_link-style-rule-and-config.md). This rule is
link *quality*, not link *style*, so it is distinct. It
`depends-on` 172 only to keep the link rules in order.

## Design

- Rule ID: MDS063 (provisional), category `prose`,
  opt-in (subjective; should not regress existing docs).
- Config:

  ```yaml
  rules:
    descriptive-link-text:
      banned: ["click here", "here", "link", "more"]
  ```

  `banned` is replace-mode (document in `ApplySettings`).
- Walk `*ast.Link`; compare trimmed lowercased text to the
  list. No autofix — only the author knows the right text.
- Skip image links and links whose text is an inline code
  span only (often an API symbol, legitimately terse).

## Tasks

1. Scaffold `internal/rules/descriptivelinktext/`.
2. Implement the AST walk and phrase comparison.
3. Implement `rule.Configurable` (`banned`, replace-mode)
   and `rule.Defaultable` returning `false`.
4. Fixture tests under the provisional
   `internal/rules/MDS063-*` directory.
5. Rule README; regenerate the docs catalog and index.
6. Add the MD059 row to the
   [linter comparison](../docs/background/markdown-linters.md).

## Acceptance Criteria

- [ ] `[click here](x)` is flagged; `[the install
      guide](x)` is clean.
- [ ] Comparison is case- and whitespace-insensitive.
- [ ] A custom `banned` list replaces the default.
- [ ] Rule is disabled by default.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes
