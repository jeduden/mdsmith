---
id: 61
title: Required Structure Rule Hardening
status: ✅
---
# Required Structure Rule Hardening

## Goal

Improve MDS020 `required-structure` reliability and diagnostics
so template-driven docs fail for real structure mismatches
with clear, actionable messages.

## Tasks

1. [x] Audit current MDS020 behavior against `rules/proto.md`
   and identify mismatch classes not covered by tests
   (heading order/level, optional sections, sync fields).
2. [x] Expand `requiredstructure` unit tests with focused fixtures
   for false positives and false negatives, including
   front matter/body sync scenarios.
3. [x] Refine matching logic for required headings and sync points
   to reduce ambiguous comparisons and improve determinism.
4. [x] Improve diagnostic messages to include expected vs actual
   structure details and precise heading context.
5. [x] Update `rules/MDS020-required-structure/README.md`
   with clarified settings, examples, and diagnostics.

## Acceptance Criteria

- [x] MDS020 correctly detects missing, reordered,
      and wrong-level required headings from template input.
- [x] Sync checks correctly validate heading/body placeholders
      against front matter fields without spurious matches.
- [x] Diagnostics include actionable expected vs actual detail
      at stable line locations.
- [x] Tests cover representative success/failure cases,
      including template config edge cases.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
