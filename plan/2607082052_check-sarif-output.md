---
id: 2607082052
title: "SARIF output format for `mdsmith check`"
status: "🔲"
model: sonnet
summary: >-
  Add `-f sarif` to `mdsmith check` (and `fix --dry-run`) so its
  diagnostics land in the same GitHub Code Scanning dashboard APM's
  `apm audit --ci -f sarif` feeds. Generalize the SARIF machinery
  the security-review engine already ships. Opportunity G-2.
depends-on: []
---
# SARIF output for `mdsmith check`

## Goal

Let `mdsmith check` emit SARIF. Its findings then
show up as GitHub Code Scanning alerts on a PR. That
is where APM's `apm audit --ci` already uploads its
own findings.

## Background

The documented APM CI pattern is `apm audit --ci
--policy org -f sarif -o report.sarif`, uploaded via
`github/codeql-action/upload-sarif`, so agent-context
findings appear as Code Scanning alerts.
[`mdsmith check`](../docs/reference/cli/check.md)
emits only `text` or `json`, so its diagnostics on
the same `.apm/` files cannot join that dashboard.
This is opportunity G-2 in the
[APM opportunity catalogue](../docs/research/apm-mdsmith/opportunities.md).

mdsmith already renders SARIF today. The
[security-review engine](../internal/secreview/sarif.go)
and the
[release audit](../internal/release/auditsarif.go)
both emit it. So the renderer exists; it is just not
wired to the check command. This plan moves those
structs into a shared package and routes the check
command's diagnostics through them. That makes the
work `partial`, not new.

## Non-Goals

- Changing the default format. `text` stays the
  default; SARIF is opt-in.
- SARIF for the LSP surface. Editor diagnostics
  already flow through LSP; this plan is the CLI
  output path.
- New diagnostic data. SARIF maps the file, line,
  column, rule id, and severity the JSON format
  already carries.

## Tasks

1. Red/green: extract the SARIF structs shared by
   `internal/secreview` and `internal/release` into a
   shared internal package with a stable rendering
   test.
2. Red/green: `formatDiagnostics` tests for a `sarif`
   format that maps each diagnostic to a SARIF 2.1.0
   result — `ruleId = MDS###`, `physicalLocation`
   region from file/line/column, `level` from
   severity.
3. Emit one `reportingDescriptor` per fired rule with
   a `helpUri` to the rule's doc page, and
   `tool.driver.name = mdsmith` with the build
   version.
4. Add `sarif` to the `-f`/`--format` value set on
   `check` and on `fix --dry-run`; update
   [check.md](../docs/reference/cli/check.md) and
   [fix.md](../docs/reference/cli/fix.md).
5. Show the GitHub Actions upload snippet in the
   coexist-with-APM guide's CI section.
6. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [ ] `mdsmith check -f sarif docs/` emits valid
      SARIF 2.1.0 with one result per diagnostic and
      correct file/line/column regions.
- [ ] Each fired rule appears once in
      `tool.driver.rules` with a `helpUri`.
- [ ] `fix --dry-run -f sarif` emits the same shape
      for the diagnostics it would fix.
- [ ] `text` remains the default when `-f` is
      omitted.
- [ ] The `check` and `fix` reference pages list the
      `sarif` value.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint
      run` reports no issues.
- [ ] `mdsmith check .` — 0 failures.
