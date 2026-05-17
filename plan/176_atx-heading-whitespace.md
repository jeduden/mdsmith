---
id: 176
title: ATX heading whitespace and indentation rule
status: "🔲"
model: sonnet
depends-on: []
summary: >-
  New rule MDS059 (provisional) covering the markdownlint
  ATX-heading whitespace family — MD018, MD019, MD020,
  MD021 — plus MD023 heading-start-left. Autofix
  normalizes the spacing and dedents the heading.
---
# ATX heading whitespace and indentation rule

## Goal

Flag malformed ATX headings: a missing or doubled space
after the opening hashes, the same on the closing hashes,
and a heading indented away from column 1. This closes the
MD018-021 and MD023 gap listed in the
[linter comparison](../docs/background/markdown-linters.md).

## Background

markdownlint splits this into five rules. mdsmith folds
them into one rule with distinct messages, because they
share the same node and the same fix pass.

- `#Heading` — MD018 no-missing-space-atx
- `#  Heading` — MD019 no-multiple-space-atx
- `#Heading#` / `# Heading  #` — MD020 / MD021 (closed ATX)
- `   # Heading` — MD023 heading-start-left

goldmark only emits an `*ast.Heading` when the syntax is
already a valid heading, so `#Heading` (no space) parses as
a paragraph. Detection therefore works on the raw line at
the heading's source position, not only the AST node.

## Design

- Rule ID: MDS059 (provisional), category `style`,
  default-enabled (these are unambiguous defects).
- For every line whose first non-space byte is `#`,
  classify the opening run, the optional closing run, and
  the leading indentation; emit one diagnostic per defect.
- Autofix rewrites the line to `#{level} {text}` with no
  leading indentation and no closed-ATX suffix unless the
  project opts into closed style (out of scope here; v1
  normalizes to open ATX).
- Skip fenced/indented code and lines inside a
  `<?...?>` directive body.

## Tasks

1. Scaffold `internal/rules/headingwhitespace/`.
2. Implement line-level detection for the five defects.
3. Implement the autofix and loop-stability test.
4. Fixture tests under the provisional
   `internal/rules/MDS059-*` directory: one bad file per
   defect plus a clean good file.
5. Rule README from the MDS012 template; regenerate the
   docs catalog and rule index.
6. Add the MD018-021 / MD023 rows to the
   [linter comparison](../docs/background/markdown-linters.md)
   structural table.

## Acceptance Criteria

- [ ] `#Heading` emits a missing-space diagnostic and
      fixes to `# Heading`.
- [ ] `#  Heading` fixes to `# Heading`.
- [ ] `# Heading #` extra-space and `#Heading#` cases are
      detected and normalized.
- [ ] An indented heading is flagged and dedented.
- [ ] Code blocks and directive bodies are never flagged.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes
