---
title: Coverage Gate
summary: Codecov coverage gate and CI status checks.
---
# Coverage Gate

Codecov reports coverage on every PR. Three CI jobs
upload reports: `test` (flag `go`),
`vscode-extension` (flag `typescript`), and
`scripts-coverage-upload` (flag `scripts`). Each
flag scopes the report to the files it measured.

## Status checks

The `project`, `patch`, and `changes` status checks
are currently disabled in `codecov.yml` (each set to
`false`). Coverage numbers still post in the codecov
PR comment. The codecov UI tracks the trend per flag
and component.

The gate is off because the main baseline does not
yet include the `scripts` flag. The strict
(`threshold: 0%`) project gate would otherwise tank
on the commit that introduces the new flag even
though net coverage rose. `informational: true` is
the documented shim for this case but has a
long-standing codecov bug on `default` blocks.

Restore the strict gate after a main merge that
covers the scripts files. Re-add the `default`
block under each status with `target: auto` and
`threshold: 0%`. Then the same three checks return:

- **project** — overall coverage must not drop below
  the base commit.
- **patch** — changed lines must hit the project
  baseline.
- **changes** — no file may lose coverage vs base.

Fork PRs skip the upload and are not gated. Add
tests for uncovered code before merging.

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
