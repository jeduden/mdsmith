---
id: 2606071930
title: Consolidate duplicated table-parse helpers in tablereadability
status: "✅"
summary: >-
  Move tablereadability's private findTables/tryParseTable
  into tablefmt so the two rules share one copy.
model: sonnet
depends-on: []
---
# Consolidate duplicated table-parse helpers

## Goal

Remove duplicated `findTables` and `tryParseTable` from
`internal/rules/tablereadability/rule.go` by sharing the
boundary-detection logic already in
`internal/rules/tablefmt`.

## Background

Both `tablereadability` and `tablefmt` carry private copies
of `findTables` and `tryParseTable`. The copies diverged on
the perf pass (`1ee98e7`). Future perf or correctness fixes
risk being applied to one copy and missed in the other.

The two `table` types differ. `tablefmt.table` stores
`cells []string` for formatting. `tablereadability.tableRow`
stores `cells [][]byte` for zero-alloc counting. A clean
consolidation exports boundary detection separately from
cell parsing. Or it accepts distinct `table` types with
a shared scanner.

Severity: tax (DRY violation; copies diverge on every perf
pass).

## Tasks

1. Export a table-boundary scanner from `tablefmt` — a
   helper that returns start/end indices of each table block
   without parsing cells.
2. Update `tablereadability` to call the exported scanner and
   parse cells itself from the detected line ranges.
3. Add or update unit tests in both packages.
4. Verify `TestRulesDoNotImportEachOther` still passes.
5. Verify no regression: `go test ./...`.

## Acceptance Criteria

- [x] Table-boundary scanning exists in one package only.
- [x] `tablereadability` does not duplicate scanning logic.
- [x] `TestRulesDoNotImportEachOther` passes.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
