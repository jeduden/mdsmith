# Taxonomy

The initial taxonomy has two categories:

- `reference`
- `other`

## `reference`

Definition: lookup-oriented technical material such as API docs,
CLI flags, specs, man-style references, and changelogs.

Examples:

- `docs/reference/cli.md`
- `api/schema.md`
- `CHANGELOG.md`

## `other`

Definition: markdown that is not primarily lookup/reference.
This includes tutorials, conceptual guides, overviews, and
process docs.

Examples:

- `guides/getting-started.md`
- `docs/concepts/architecture.md`

## Classification Heuristic

The current classifier labels a record as `reference` when at
least one signal matches:

- path contains `reference`, `api`, `/spec/`, `/specification/`, or `man`
- filename contains `reference`, `api`, `spec`, `schema`,
  `config`, `changelog`, or `release-notes`
- first markdown heading contains `reference`, `api`,
  `specification`, `changelog`, `command`, or `options`

Records that do not match these signals are labeled `other`.
