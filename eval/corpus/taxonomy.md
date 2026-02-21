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

## Request for Comments (RFC)

- Definition: pre-decision proposal for review.
- Boundary rule: classify when feedback is requested and open
  questions remain.
- Positive example: `RFC-012-token-budget.md`.
- Negative example: `docs/adr/0001-config-format.md`.

## Design Proposal / Tradeoff Memo

- Definition: option analysis with recommendation.
- Boundary rule: classify when alternatives and tradeoffs are
  explicitly compared.
- Positive example: `plan/62_corpus-acquisition.md`.
- Negative example: `docs/api/reference.md`.

## Runbook / Playbook

- Definition: operational execution procedure.
- Boundary rule: classify when a responder can execute it in
  sequence during operations.
- Positive example: `ops/runbook/release.md`.
- Negative example: `postmortems/2026-02-outage.md`.

## Incident Postmortem

- Definition: retrospective on incident cause and actions.
- Boundary rule: classify when the document centers one
  incident timeline.
- Positive example: `postmortems/2026-02-outage.md`.
- Negative example: `ops/runbook/release.md`.

## Changelog / Release Notes / Migration Guide

- Definition: versioned change logs and upgrade notes.
- Boundary rule: classify when changes are organized by
  release or version.
- Positive example: `CHANGELOG.md`.
- Negative example: `docs/faq.md`.

## Project / Process Docs

- Definition: repository governance and contributor process.
- Boundary rule: classify when content is project-level process
  or orientation.
- Positive example: `README.md` or `CONTRIBUTING.md`.
- Negative example: `docs/spec/config.md`.

## API / CLI / Configuration / Specification

- Definition: interface and contract definitions.
- Boundary rule: classify when it describes flags, request
  fields, config keys, or schemas.
- Positive example: `docs/cli/flags.md`.
- Negative example: `docs/tutorial/getting-started.md`.

## Troubleshooting / FAQ / Onboarding / Glossary

- Definition: quick diagnosis and orientation references.
- Boundary rule: classify when it maps symptoms to fixes or
  terms to definitions.
- Positive example: `docs/troubleshooting/common-errors.md`.
- Negative example: `CHANGELOG.md`.
