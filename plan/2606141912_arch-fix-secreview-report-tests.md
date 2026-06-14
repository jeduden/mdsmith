---
id: 2606141912
title: Add per-function unit tests for secreview/report.go
status: "🔲"
summary: >-
  internal/secreview/report.go has 13 functions
  with no dedicated TestFunctionName entries in
  a report_test.go. Add a report_test.go with
  one named test per function.
model: haiku
depends-on: []
---
# Add per-function unit tests for secreview/report.go

## Context

Closes audit entry "tax — missing per-function
named tests in `internal/secreview/report.go`"
from the [2026-06-14 audit][audit].

[audit]: ../docs/development/architecture-audit.md

## Goal

Add `internal/secreview/report_test.go`.
One `TestFunctionName` per function.
Go arch §"Go-specific bindings"
names this pattern.

## Tasks

1. Create `internal/secreview/report_test.go`
   in `package secreview`.
2. Add one test function per production
   function in `report.go`:

  - `TestOrQuestion`
  - `TestBuildReport`
  - `TestWriteHeader`
  - `TestWriteSummary`
  - `TestTableCell`
  - `TestSeverityCounts`
  - `TestWriteFindingSections`
  - `TestRenderFinding`
  - `TestWriteFindingProse`
  - `TestWriteCoverage`
  - `TestCapitalize`
  - `TestBuildAnnotations`
  - `TestAnnotationBody`

3. Each test must drive the function
   directly (not through `Render`).
4. Run `go test ./internal/secreview/...`.

## Acceptance Criteria

- [ ] `internal/secreview/report_test.go`
  exists with one `Test<Name>` per function.
- [ ] Each test exercises the function in
  isolation with inline string fixtures.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports
  no issues.
