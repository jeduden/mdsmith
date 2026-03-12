---
id: 62
title: Corpus Acquisition and Taxonomy
status: ✅
---
# Corpus Acquisition and Taxonomy

## Goal

Create a repeatable workflow to collect and label Markdown files.
The corpus should cover agent docs, technical docs, and architecture
decision artifacts while tracking license and source metadata.

## Tasks

1. Define the corpus taxonomy and labeling rules.
2. Define source inclusion/exclusion policy
   (license allowlist, repository quality, generated content filters).
3. Implement a collection pipeline that gathers Markdown files and
   records provenance metadata
   (repository, path, commit SHA, license, collection date).
4. Implement normalization and quality gates
   (format normalization, minimum content thresholds, near-duplicate
   detection, generated-file exclusion).
5. Implement a sampling strategy to balance categories and avoid
   README-heavy skew.
6. Create a manual QA pass on a stratified sample and refine taxonomy
   rules from observed confusion cases.
7. Freeze a versioned dataset manifest and deterministic
   train/dev/test splits.
8. Document a periodic refresh workflow with drift reporting by
   category.

## Taxonomy Scope

- Agent-control docs (`AGENTS.md`, `CLAUDE.md`, skills, task prompts)
- Tutorial
- How-to guide
- Reference
- Explanation/background
- Architecture decision record (ADR)
- Request for comments (RFC)
- Design proposal/tradeoff memo
- Runbook/playbook
- Incident postmortem
- Changelog/release notes/migration guide
- Project/process docs (`README`, `CONTRIBUTING`, `SECURITY`, governance)
- API/CLI/configuration/specification docs
- Troubleshooting/FAQ/onboarding/glossary

## Acceptance Criteria

- [x] Taxonomy includes category definitions, boundary rules, and at
      least one positive/negative example per category.
- [x] Collection pipeline produces a manifest with source provenance and
      license metadata for every file.
- [x] Dataset build excludes generated/low-signal content and reports
      deduplication statistics.
- [x] Final corpus is category-balanced within defined target ranges and
      reports per-category counts.
- [x] Manual QA on stratified samples reports precision/recall or
      agreement metrics and drives at least one documented taxonomy
      refinement.
- [x] Refresh process is documented and can publish a versioned corpus
      update with drift summary.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
