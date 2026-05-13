---
id: 154
title: 'arch-fix: extract cross-rule helpers'
status: "🔲"
summary: >-
  Move shared fence-position and table-format
  helpers out of donor rule packages into
  sibling helper packages so no rule imports
  another rule.
model: "sonnet"
depends-on: []
---
# arch-fix: extract cross-rule helpers

## Goal

Stop rules importing other rules. The
[architecture hub](../docs/development/architecture/index.md)
forbids it. Keep the boundary at compile
time.

## Context

Closes the first blocker in
[Audit 2026-05-13](../docs/development/architecture-audit.md).

Five rules reach across the rule
boundary for shared helpers today.

Four rules import
`internal/rules/fencedcodestyle`. Each
uses `FenceCharAt`, `FenceOpenLine`,
`FenceOpenLineRange`, `FenceCloseLine`,
and `FenceCloseLineRange`. The four
rules are:

- `internal/rules/fencedcodelanguage`
- `internal/rules/orderedlistnumbering`
- `internal/rules/unclosedcodeblock`
- `internal/rules/blanklinearoundfencedcode`

A fifth rule (`internal/rules/catalog`)
imports `internal/rules/tableformat`
for `FormatString`.

The donor rule packages still own
MDS010 and MDS035. The cross-rule
reach is into their exported helper
functions, not their `rule.Rule`
implementation. The structural fix is
the same for both. Lift the helpers
into a sibling helper package
consumed by donor and consumer.

## Tasks

1. Create `internal/rules/fencepos/`
   exporting `CharAt`, `OpenLine`,
   `OpenLineRange`, `CloseLine`, and
   `CloseLineRange`. Drop the `Fence`
   prefix; the package name carries
   the noun. Move the implementations
   out of
   `internal/rules/fencedcodestyle/rule.go`.
2. Update
   `internal/rules/fencedcodestyle/rule.go`
   to import and consume `fencepos`.
   No exported helpers remain on the
   rule package.
3. Update the four consumer rules
   (`fencedcodelanguage`,
   `orderedlistnumbering`,
   `unclosedcodeblock`,
   `blanklinearoundfencedcode`) to
   import `fencepos` instead of
   `fencedcodestyle`.
4. Create `internal/rules/tablefmt/`
   exporting `FormatString`. Move the
   implementation out of
   `internal/rules/tableformat/rule.go`.
5. Update
   `internal/rules/tableformat/rule.go`
   and
   `internal/rules/catalog/rule.go`
   to import `tablefmt`.
6. Add a grep-based regression test
   under `internal/integration/`.
   Make it fail if any non-test file
   under `internal/rules/` imports
   another `internal/rules/<...>/`
   package. Allow only the documented
   helpers (`astutil`, `settings`,
   `fencepos`, `tablefmt`) and
   same-rule sub-packages.

## Acceptance Criteria

- [ ] `internal/rules/fencepos/` and
  `internal/rules/tablefmt/` exist.
  Their package comments name the
  single question each answers. (SRP)
- [ ] The regression search reports
  only the allowed cross-package
  imports under `internal/rules/`.
  (DIP)
- [ ] All tests pass:
  `go test ./...`.
- [ ] `go tool golangci-lint run`
  reports no issues.
- [ ] `mdsmith check .` passes after
  the refactor.
- [ ] The audit entry for this
  blocker moves to a "Resolved by
  plan/154" section in
  [the audit log](../docs/development/architecture-audit.md).
