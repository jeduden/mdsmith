---
title: Coverage Gate
summary: Codecov coverage gate and CI status checks.
---
# Coverage Gate

Codecov blocks PRs that decrease per-file statement
coverage. Fork PRs skip the upload and are not gated.
Three status checks run on same-repo PRs:

- **project** — overall coverage must not drop below
  the base commit.
- **patch** — changed lines must have coverage at
  least equal to the project baseline.
- **changes** — no individual file's coverage may
  decrease vs the base commit.

If any check fails, Codecov posts a comment listing
the affected files with baseline, current, and delta
percentages. Fix regressions by adding tests for the
uncovered code paths before merging.

Configuration lives in `codecov.yml` at the repo
root. Two CI jobs upload reports — `test` under flag
`go` and `vscode-extension` under flag `typescript`.
Each flag scopes the report to its language's files.

## Branch and function coverage

Statement coverage does not track which branches of
an `if` or `switch` were taken.
[gobco](https://github.com/rillig/gobco) provides
condition-level branch coverage:

```bash
go tool gobco ./...
```

Flags:

- `-branch` — report branch coverage instead of
  condition coverage.
- `-list-all` — include fully-covered conditions in
  the output (default: only partially-covered).
- `-stats file.json` — persist results across runs to
  track progress over time.

## Reproducing CI statement coverage locally

```bash
mkdir -p e2e-cover
E2E_COVERDIR=e2e-cover \
  go test -covermode=atomic \
  -coverprofile=unit.cov ./...
head -1 unit.cov > merged.cov
tail -n +2 unit.cov \
  | grep -v 'cmd/mdsmith/' >> merged.cov || true
tail -n +2 e2e-cover/e2e_coverage.txt \
  | grep 'cmd/mdsmith/' >> merged.cov
go tool cover -func=merged.cov
```

Unit tests cannot cover `cmd/mdsmith/` because those
functions run in a subprocess. The merge replaces
those zero-count unit lines with the e2e counts. CI
performs additional validation (mode header match,
file existence); see the `test` job in
`.github/workflows/ci.yml`.
