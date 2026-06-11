---
title: "Quality you can verify"
summary: >-
  CI badge, Go Report Card grade, and Codecov coverage badge report
  live project health. mdsmith lints its own docs with the rules it
  ships, and a coverage gate blocks any merge that drops below the
  line.
icon: shield-check
weight: 19
group: "Built for your pipeline"
---
# Quality you can verify

The three badges at the top of the README are not decoration.
Each one is a live signal of project health.

**CI** — the `main` branch workflow must be green before any commit
lands. The badge reflects the last run.

**Go Report Card** — static analysis across all Go source. mdsmith
holds an A+ grade: no vet warnings, no lint issues beyond the ones
already tracked as known tech-debt.

**Codecov** — coverage is measured on every push, and new code
must arrive covered.

Quality is enforced, not hoped for. mdsmith lints its own docs
with the same rules it ships, so the tool eats its own cooking. A
coverage gate blocks any merge that drops coverage below the line.
