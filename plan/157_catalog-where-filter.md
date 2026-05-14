---
id: 157
title: 'Catalog filter by front matter property'
status: '🔳'
summary: >-
  Extend the catalog directive with a filter that
  selects matched files by a front matter property
  value. Enables listings filtered by nature
  (directive rule, fixable rule, ready vs draft
  plan, user-invocable skill).
model: 'sonnet'
depends-on: [19]
---
# Catalog filter by front matter property

## ...

<?allow-empty-section?>

## Goal

`<?catalog?>` selects files by glob and renders
each as a row. It cannot select rows by a front
matter property today. The driving use case is a
listing of every directive rule (a property like
`nature: directive` on the four directive rule
READMEs under [internal/rules/](../internal/rules/)).
The same shape applies to fixable rules, ready vs
draft plans, and user-invocable skills.

## ...

<?allow-empty-section?>

## Background

The catalog rule lives at
[internal/rules/catalog/](../internal/rules/catalog/).
Today its parameters are `glob`, `sort`, `row`,
`header`, `footer`, `empty`, `columns`,
`gitignore`. Front matter values flow into the
row template as `{field}` placeholders, but no
parameter narrows which files render.

The `mdsmith list query` subcommand already
selects files by a CUE expression on front
matter. See the
[query reference](../docs/reference/cli/query.md).
This plan reuses that grammar inside the catalog
directive so both surfaces stay consistent.

## ...

<?allow-empty-section?>

## Tasks

1. Add a `where:` parameter to the catalog
   directive. The value is a CUE expression
   evaluated against each matched file's parsed
   front matter. Reuse the matcher from
   [`internal/query/`](../internal/query/).
   Files whose front matter fails the expression
   are dropped before sort and render.

2. Add a `nature` property convention to rule
   READMEs. Allowed values are at least
   `directive`, `content`, `structural`,
   `meta`. Document the convention in the
   [MDS019 README](../internal/rules/MDS019-catalog/README.md)
   and the
   [rule-readme proto](../internal/rules/proto.md).

3. Backfill `nature` on every rule README and
   the matching front matter schema check in
   the rule-readme kind.

4. Surface a filtered listing in the rule
   directory at
   [internal/rules/index.md](../internal/rules/index.md):
   a section titled "Directive rules" using
   `where: 'nature == "directive"'`. Mention the
   sibling "All rules" catalog already on the
   page.

5. Document the new parameter in the
   [generating-content guide](../docs/guides/directives/generating-content.md).
   Include a worked example and the failure
   modes (unknown field, type mismatch).

6. Add fixture coverage under MDS019. Cover a
   filter that keeps a subset. Cover an invalid
   expression that triggers a clear diagnostic.

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [x] `<?catalog?>` accepts a `where:` parameter
  with a CUE expression evaluated against each
  file's front matter.
- [x] A file whose front matter does not satisfy
  the expression is excluded from the rendered
  body.
- [x] An invalid `where:` expression emits an
  MDS019 diagnostic naming the offending token
  and line.
- [ ] Every rule README carries a `nature` front
  matter key matching the documented vocabulary.
- [ ] [internal/rules/index.md](../internal/rules/index.md)
  has a "Directive rules" section using
  `where: 'nature == "directive"'` that lists
  only the four directive rules.
- [x] The
  [generating-content guide](../docs/guides/directives/generating-content.md)
  documents `where:` with a worked example.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no
  issues.

## ...

<?allow-empty-section?>
