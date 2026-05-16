---
id: 162
title: Split the overloaded `meta` rule category
status: "✅"
model: sonnet
depends-on: [142]
summary: >-
  Reclassify the 23 rules currently using
  `Category() == "meta"` into narrower buckets
  (`prose`, `structural`, `directive`) so the
  category surface usefully partitions the rule
  set.
---
# Split the overloaded `meta` rule category

## Goal

Make `Category()` useful for grouping. Today `meta`
is a 23-rule catch-all. The bucket mixes
prose-quality checks, directive validators, and
project-level invariants. That breadth defeats any
help-text grouping built on the category.

## Background

Plan 142 added MDS055–MDS058 and put them in
`prose`. They sit next to `proper-names` (MDS050).
Many other `meta` rules belong elsewhere too — some
are directive validators, others enforce
project-level invariants. The mapping table below
assigns each rule to its narrowest fit.

`nature:` (introduced on main in #274) classifies
each rule README as one of `directive | generator |
content | style | structure`. That axis is
orthogonal to `Category()`. `nature:` describes the
kind of check. `Category()` describes the document
target (line, heading, list, …). The mapping below
keeps the two axes distinct.

## Non-Goals

- Renaming the existing narrow categories
  (`code`, `heading`, `line`, `link`, `list`,
  `whitespace`, `table`, `accessibility`).
- Changing `Category()` for any non-`meta` rule.
- Adding category enforcement to the rule
  interface. `Category()` stays a free-form
  string. Only the writer-hint comment in
  [`internal/rules/proto.md`](../internal/rules/proto.md)
  documents the allowed set.

## Design

### Proposed mapping

| Rule                             | New category | Reason                                 |
|----------------------------------|--------------|----------------------------------------|
| `ambiguous-emphasis`             | `prose`      | How prose uses emphasis.               |
| `build`                          | `directive`  | Validates `<?build?>`.                 |
| `catalog`                        | `directive`  | Validates `<?catalog?>`.               |
| `conciseness-scoring`            | `prose`      | Paragraph-level metric.                |
| `cross-file-reference-integrity` | `link`       | Verifies link targets resolve.         |
| `directory-structure`            | `structural` | Allows / forbids files by directory.   |
| `duplicated-content`             | `prose`      | Compares paragraph prose across files. |
| `emphasis-style`                 | `prose`      | Emphasis delimiter choice in prose.    |
| `git-hook-sync`                  | `structural` | Project invariant on `.git/hooks/`.    |
| `include`                        | `directive`  | Validates `<?include?>`.               |
| `markdown-flavor`                | `structural` | Syntax surface used by the project.    |
| `max-file-length`                | `structural` | Project-level cap on file size.        |
| `no-inline-html`                 | `structural` | Constrains the syntax surface.         |
| `paragraph-readability`          | `prose`      | Per-paragraph readability metric.      |
| `paragraph-structure`            | `prose`      | Caps sentence / word counts in prose.  |
| `recipe-safety`                  | `directive`  | Validates `<?build?>` recipes.         |
| `required-structure`             | `structural` | Schema-based document shape.           |
| `toc`                            | `directive`  | Generated TOC freshness.               |
| `toc-directive`                  | `directive`  | Renderer-specific TOC directives.      |
| `token-budget`                   | `prose`      | Caps token estimate of prose.          |

After the move, `meta` should be empty. The
allowed set in
[`internal/rules/proto.md`](../internal/rules/proto.md)
drops `meta` and adds `directive`, `structural`. A
rule that still wants `meta` keeps it with a
rationale comment.

### Surfaces that read `Category()`

`Category()` flows into `mdsmith help`, the rule
directory README, and any LSP-side grouping. Each
surface needs a re-audit after the move so its
grouping reads sensibly. The rule-directory catalog
row uses `{name}` but not `{category}` today; adding
it is optional follow-up.

### Tests

Each rule has a `TestCategory` in its `rule_test.go`.
The cleanup updates every assertion. The
[integration runner](../internal/integration/rules_test.go)
does not assert on `Category()`, so fixtures stay
unchanged.

## Tasks

1. Switch `Category()` to the new bucket for each
   rule in the mapping table. Update the matching
   `TestCategory` and the README's
   Meta-Information bullet.
2. Update
   [`internal/rules/proto.md`](../internal/rules/proto.md)'s
   writer-hint comment: drop `meta`, add
   `directive` and `structural`.
3. Sweep
   [`internal/rules/index.md`](../internal/rules/index.md)
   and confirm the catalog still renders cleanly.
4. Re-run `mdsmith help <rule>` for one rule per
   new category and confirm the surface text reads
   sensibly.

## Acceptance Criteria

- [x] No rule reports `Category() == "meta"`
      (unless a follow-up rationale documents why
      it remains).
- [x] Each affected rule's README
      Meta-Information bullet matches the new
      value.
- [x] `internal/rules/proto.md`'s writer-hint
      comment lists exactly the categories used
      in production code.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
