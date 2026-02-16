# Source Policy

This policy defines what is eligible for corpus collection.

## Inclusion Rules

- Repository license must be in the allowlist.
- Repository must pass quality gates:
  - minimum stars,
  - minimum recent commits,
  - not archived,
  - CI enabled when required.
- File extension must be `.md` or `.markdown`.
- File must match source include globs and not match source
  exclude globs.

## Exclusion Rules

- Generated paths are excluded:
  `vendor`, `node_modules`, `dist`, `build`, `generated`, `gen`.
- Generated text markers are excluded:
  `code generated` and `do not edit`.
- Low-signal files are excluded when below
  `min_words` or `min_chars` thresholds.
- Exact duplicates are dropped by normalized content hash.
- Near duplicates are dropped by token-set Jaccard threshold.

## Provenance Requirements

Every kept file must record:

- source name,
- repository,
- repository URL,
- relative file path,
- commit SHA,
- source license,
- collection date,
- normalized content hash.

## Balancing Rules

- Cap README share with `max_readme_share`.
- Apply per-category min and max target ranges.
- Record violations in `report.json`.
