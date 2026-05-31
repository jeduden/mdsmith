---
id: 221
title: Add unit tests for pkg/mdsmith private helpers
status: "🔲"
summary: >-
  Five private helpers in the new public
  pkg/mdsmith package lack dedicated unit
  tests. Add TestCapabilityList,
  TestFrontMatterEnabled, TestFirstError,
  TestCloneDiagnostics, and TestIndexSlash.
model: ""
depends-on: []
---
# Add unit tests for pkg/mdsmith private helpers

## Goal

[pkg/mdsmith](../pkg/mdsmith/) is a new
public cross-system contract. Its six
exported `Session` methods are fully
covered. Five unexported helpers have
no dedicated unit test, violating the
[test pyramid rule](../docs/development/architecture/tests.md).

Uncovered functions (all production code,
not test-only):

- `capabilityList()` in
  `pkg/mdsmith/session.go`
- `frontMatterEnabled()` in
  `pkg/mdsmith/session.go`
- `firstError()` in
  `pkg/mdsmith/session.go`
- `cloneDiagnostics()` in
  `pkg/mdsmith/session.go`
- `indexSlash()` in
  `pkg/mdsmith/workspace.go`

## Tasks

1. Add `TestCapabilityList` to
   `pkg/mdsmith/session_api_test.go`.
   Assert the returned slice matches the
   expected capability names in sorted
   order.
2. Add `TestFrontMatterEnabled` to
   `pkg/mdsmith/session_api_test.go`.
   Test enabled (config with strip-
   frontmatter: true) and disabled.
3. Add `TestFirstError` to
   `pkg/mdsmith/session_api_test.go`.
   Test nil slice, all-nil, first non-nil.
4. Add `TestCloneDiagnostics` to
   `pkg/mdsmith/session_api_test.go`.
   Assert the clone is a distinct slice;
   mutating it does not touch the source.
5. Add `TestIndexSlash` to
   `pkg/mdsmith/workspace_test.go`.
   Cover no slash, slash at 0, slash in
   middle, slash at end.
6. Run `go test ./pkg/mdsmith/...`.

## Acceptance Criteria

- [ ] `TestCapabilityList` exists and
  passes.
- [ ] `TestFrontMatterEnabled` exists and
  passes.
- [ ] `TestFirstError` exists and passes.
- [ ] `TestCloneDiagnostics` exists and
  passes.
- [ ] `TestIndexSlash` exists and passes.
- [ ] `go test ./pkg/mdsmith/...` green.
- [ ] `go tool golangci-lint run` clean.
