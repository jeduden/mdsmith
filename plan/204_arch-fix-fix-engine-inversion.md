---
id: 204
title: Fix internal/fix importing internal/engine
status: "🔲"
summary: >-
  internal/fix/fix.go imports internal/engine for
  CheckRules, ConfigureRule, and DedupeDiagnostics.
  The layering map places engine above fix. Move
  the three functions to a lower shared package.
model: ""
depends-on: []
---
# Fix internal/fix importing internal/engine

## Goal

[internal/fix/fix.go](../internal/fix/fix.go) imports
[internal/engine](../internal/engine) for three functions.
The layering map places engine above fix.
The import inverts that arrow. Moving
the three functions to a package both
engine and fix can import restores the
intended direction.

## Tasks

1. Read `CheckRules`, `ConfigureRule`,
   and `DedupeDiagnostics` and identify
   their dependencies.
2. Choose the target package (`internal/lint`,
   `internal/rule`, or a new package).
   Document the choice here.
3. Move the three functions.
4. Update all callers in `internal/engine`
   and `internal/fix`.
5. Verify `internal/fix` no longer
   imports `internal/engine`.
6. Add a contract test in
   `internal/integration/` that fails if
   `internal/fix` imports `internal/engine`.
7. Run `go build ./...` and
   `go test ./...`.

## Acceptance Criteria

- [ ] `grep -r --include='*.go' '".*internal/engine"' internal/fix/`
  returns nothing.
- [ ] A contract test guards the boundary.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` clean.
