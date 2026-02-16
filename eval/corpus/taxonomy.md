# Corpus Taxonomy

This taxonomy defines category labels for the corpus pipeline.
Each category has a definition, boundary rule, and one
positive and negative example.

## Agent-Control Docs

- Definition: instructions that directly constrain agent behavior.
- Boundary rule: classify only when the document issues direct
  operating rules for agents.
- Positive example: `AGENTS.md`.
- Negative example: `README.md`.

## Tutorial

- Definition: guided learning sequence for new users.
- Boundary rule: classify when the doc teaches step-by-step with
  learning intent.
- Positive example: `docs/tutorial/getting-started.md`.
- Negative example: `docs/cli/flags.md`.

## How-To Guide

- Definition: task-focused instructions for a specific goal.
- Boundary rule: classify when the outcome is operational
  completion, not conceptual learning.
- Positive example: `docs/how-to/rotate-keys.md`.
- Negative example: `guides/why-token-budget-matters.md`.

## Reference

- Definition: lookup material with factual mappings.
- Boundary rule: classify when users scan for values, fields,
  or definitions.
- Positive example: `internal/rules/MDS001-line-length/README.md`.
- Negative example: `plan/62_corpus-acquisition.md`.

## Explanation / Background

- Definition: rationale and conceptual context.
- Boundary rule: classify when the primary purpose is why and
  tradeoff context.
- Positive example: `guides/metrics-tradeoffs.md`.
- Negative example: `ops/runbook/deploy.md`.

## Architecture Decision Record (ADR)

- Definition: decision log with context and consequences.
- Boundary rule: classify when one concrete architecture
  decision is recorded.
- Positive example: `docs/adr/0001-config-format.md`.
- Negative example: `RFC-012-token-budget.md`.

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
