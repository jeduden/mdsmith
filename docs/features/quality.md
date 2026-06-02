---
title: "Quality you can verify"
summary: >-
  CI badge, Go Report Card grade, and Codecov coverage badge report
  live project health. mdsmith lints its own docs with the rules it
  ships, and a coverage gate blocks any merge that drops below the
  line.
icon: shield-check
link: "/development/coverage/"
weight: 18
---
# Quality you can verify

The three badges at the top of the README are not decoration.
Each one is a live signal of project health.

**CI** — the `main` branch workflow must be green before any commit
lands. The badge reflects the last run.

**Go Report Card** — static analysis across all Go source. mdsmith
holds an A+ grade: no vet warnings, no lint issues beyond the ones
already tracked as known tech-debt.

**Codecov** — coverage is measured on every push. The gate rejects
merges that drop the project below 0.2% of current coverage, and
any net-new code added by a patch must be covered (0% tolerance on
the diff). Per-file coverage decreases are also tracked.

Quality is enforced, not hoped for. mdsmith lints its own docs
with the same rules it ships, so the tool eats its own cooking. A
[coverage gate](../development/coverage.md) blocks any merge that
drops coverage below the line.

See the [coverage gate doc](../development/coverage.md) for
thresholds and CI status checks.
