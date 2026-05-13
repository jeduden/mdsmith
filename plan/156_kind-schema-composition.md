---
id: 156
title: 'Composable required-structure schemas across multiple kinds'
status: '🔲'
summary: >-
  Two kinds whose required-structure schemas
  differ overwrite each other under deep-merge.
  This plan adds composition so a
  directive-rule-readme kind layers on top of
  rule-readme without losing either constraint
  set.
model: 'opus'
depends-on: [97, 146]
---
# Composable required-structure schemas across kinds

## ...

<?allow-empty-section?>

## Goal

A file resolved by N kinds gets one effective
required-structure schema. That schema must be the
composition of each kind. Today the last-written
schema wins.

The driving use case is the new
directive-rule-readme kind from PR #274. It must
layer a required Pattern section onto rule-readme.
The same shape applies later to runbook + on-call
or plan + epic pairs.

## ...

<?allow-empty-section?>

## Background

The config merge layer in `internal/config/`
already deep-merges rule settings across layers
(defaults, kinds, overrides). Scalar replacement
plus map-key merging covers most rules. See
[merge.go](../internal/config/merge.go) for the
implementation.

required-structure is the outlier. Its setting is
either `schema:` (a path to a `proto.md` file) or
`inline-schema:` (a parsed Schema). Two kinds that
both set `schema:` deep-merge as scalars and the
second wins. One kind sets `schema:` and another
sets `inline-schema:`: MDS020 today picks one and
drops the other. Neither path composes.

## ...

<?allow-empty-section?>

## Tasks

1. Document the current behaviour. Add a short
   note to the [cross-system
   doc](../docs/development/architecture/cross-system.md)
   stating that conflicting required-structure
   schemas across kinds pick the last applied.
   That is the baseline this plan moves off of.

2. Decide the composition rule. Three candidates:

  - Concatenate sections; intersect frontmatter
    types. Sections from earlier kinds run first.
    Later kinds append. A key required by any kind
    is required. The stricter `closed:` wins.
  - Schema-include. The later kind's schema gets a
    synthetic `<?include?>` of the earlier kind's
    schema at the top. Composition piggy-backs on
    the schema-include feature in
    [MDS020](../internal/rules/MDS020-required-structure/README.md).
  - Explicit extends. Each kind declares
    `extends: <kind-name>` in its schema body. The
    merge resolver walks the chain. Mirrors
    [plan/135_schema-extends.md](135_schema-extends.md)
    at the kind level.

  Acceptance test for the decision. Write a YAML
  doc with two kinds A and B. A requires `## Goal`.
  B requires `## Risks`. After composition a file
  resolving to `[A, B]` must require both.

3. Implement the chosen rule in the merge layer.
   Special-case `required-structure.schema` and
   `required-structure.inline-schema`. They run
   the composition procedure. Other paths keep
   the default scalar replacement. Add a test in
   `internal/config/` covering two kinds with
   disjoint required headings.

4. Validate directive-rule-readme + rule-readme.
   Reassign the four directive READMEs through
   both kinds in [.mdsmith.yml](../.mdsmith.yml).
   Confirm `mdsmith check .` enforces both header
   sets. Remove the duplicated headings from
   `internal/rules/directive-proto.md` so it
   declares only Pattern additions.

5. Document the composition rule. Update the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md)
   and [docs/guides/schemas.md](../docs/guides/schemas.md).
   Include a worked two-kind example.

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] A file resolving to two kinds with disjoint
  required sections fails `mdsmith check` until
  both sets are present.
- [ ] A file resolving to two kinds with disjoint
  required front-matter keys fails `mdsmith check`
  until both sets are present.
- [ ] `internal/rules/directive-proto.md` no
  longer duplicates rule-readme's `Config`,
  `Examples`, and `Meta-Information` headings; it
  declares only Pattern additions.
- [ ] [docs/guides/schemas.md](../docs/guides/schemas.md)
  documents the composition rule with a two-kind
  worked example.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no
  issues.

## ...

<?allow-empty-section?>
