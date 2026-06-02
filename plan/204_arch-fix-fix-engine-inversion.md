---
id: 204
title: Fix internal/fix importing internal/engine
status: "✅"
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

## Target package choice

- `DedupeDiagnostics` moved to `internal/lint`:
  it only operates on `lint.Diagnostic` values and
  has no other dependencies.
- `CheckRules` and `ConfigureRule` moved to a new
  `internal/checker` package: both need `config.RuleCfg`,
  `rule.Rule`, and `lint.File`. The `rule` package
  already imports `config` (which imports `rule`), so
  adding `config` to `rule` would cycle. A new
  `internal/checker` package sits below both
  `internal/engine` and `internal/fix` with no
  import of either.

## Tasks

1. [x] Read `CheckRules`, `ConfigureRule`,
   and `DedupeDiagnostics` and identify
   their dependencies.
2. [x] Choose the target package (`internal/lint`,
   `internal/rule`, or a new package).
   Document the choice here.
3. [x] Move the three functions.
4. [x] Update all callers in `internal/engine`
   and `internal/fix`.
5. [x] Verify `internal/fix` no longer
   imports `internal/engine`.
6. [x] Add a contract test in
   `internal/integration/` that fails if
   `internal/fix` imports `internal/engine`.
7. [x] Run `go build ./...` and
   `go test ./...`.

## Acceptance Criteria

- [x] `grep -r --include='*.go' '".*internal/engine"' internal/fix/`
  returns nothing.
- [x] A contract test guards the boundary.
- [x] `go build ./...` clean.
- [x] `go test ./...` passes.
- [x] `go tool golangci-lint run` clean.
