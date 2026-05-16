---
title: "Quality you can verify"
summary: >-
  The README build, Go Report Card, and coverage badges report live
  project health. mdsmith lints its own docs with the rules it ships,
  and a coverage gate blocks merges that drop below the line.
icon: shield-check
link: "/docs/development/coverage/"
weight: 8
---
# Quality you can verify

The three badges at the top of the README are not decoration.
Each one is a live signal of project health.

The build badge tracks the CI workflow on `main`. The Go Report
Card badge grades the Go source. The coverage badge reports test
coverage from Codecov.

Quality is enforced, not hoped for. mdsmith lints its own docs
with the same rules it ships, so the tool eats its own cooking. A
[coverage gate](../development/coverage.md) blocks any merge that
drops coverage below the line.

See the [coverage gate
doc](../development/coverage.md) for the threshold and CI status
checks.
