---
id: 2606141910
title: Extract build path helpers out of internal/rules/build
status: "🔲"
summary: >-
  internal/build/builder.go imports
  internal/rules/build for three path helpers,
  violating the rule that packages outside
  internal/rules/ must not import a specific
  rule package. Extract the helpers into a new
  sibling package internal/rules/buildpathutil.
model: sonnet
depends-on: []
---
# Extract build path helpers out of internal/rules/build

## Context

Closes audit entry "tax — `internal/build` imports
`internal/rules/build`" from the
[2026-06-14 audit][audit].

[audit]: ../docs/development/architecture-audit.md

## Goal

Remove the DIP violation.
[`internal/build/builder.go`][builder]
imports `internal/rules/build` for three
helpers. Go arch doc §"Common violations
to flag" forbids this: only packages
inside `internal/rules/` may import a
specific rule package.

[builder]: ../internal/build/builder.go

## Tasks

1. Create `internal/rules/buildpathutil/`
   with `buildpathutil.go` and
   `buildpathutil_test.go`.
2. Move `CheckGlobMatchCap` and
   `ResolvePathInRoot` from
   `internal/rules/build/resolve.go` to the
   new package. Move `UnderMdsmithDir` from
   `internal/rules/build/rule.go` to the new
   package.
3. Update `internal/rules/build` to import
   from `buildpathutil` instead of defining
   those functions.
4. Update `internal/build/builder.go` to
   import `internal/rules/buildpathutil`
   instead of `internal/rules/build`.
5. Add `buildpathutil` to the
   `allowedRuleHelpers` map in
   `internal/integration/rule_boundaries_test.go`.
6. Add a new contract test that asserts
   `internal/build` does not import any
   `internal/rules/<name>` package other
   than `buildpathutil`.
7. Run `go test ./...` and
   `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `internal/build/builder.go` imports
  `internal/rules/buildpathutil`, not
  `internal/rules/build`.
- [ ] `internal/rules/buildpathutil` has a
  dedicated `buildpathutil_test.go` covering
  each function.
- [ ] The new contract test in
  `internal/integration/` fails if
  `internal/build` imports any
  `internal/rules/<name>` except
  `buildpathutil`.
- [ ] `TestRulesDoNotImportEachOther` still
  passes.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports
  no issues.
