---
id: 2606141911
title: Remove deprecated engine wrappers for checker and lint
status: "🔲"
summary: >-
  internal/engine/check.go and runner.go hold
  deprecated thin wrappers over
  internal/checker and internal/lint. No
  production caller remains. Remove the wrappers,
  update the one test caller, and fix stale
  comments.
model: haiku
depends-on: []
---
# Remove deprecated engine wrappers for checker and lint

## Context

Closes audit entry "tax — deprecated wrappers
remain in `internal/engine`" from the
[2026-06-14 audit][audit].

[audit]: ../docs/development/architecture-audit.md

## Goal

Delete the deprecated forwarding functions from
`internal/engine` and update every call site.

The wrappers and their intended replacements:

- `engine.ConfigureRule` →
  `checker.ConfigureRule`
  (`internal/engine/check.go`)
- `engine.CheckRules` →
  `checker.CheckRules`
  (`internal/engine/check.go`)
- `engine.DedupeDiagnostics` →
  `lint.DedupeDiagnostics`
  (`internal/engine/runner.go`)

## Tasks

1. Delete `ConfigureRule` and `CheckRules`
   from `internal/engine/check.go`.
2. Delete `DedupeDiagnostics` from
   `internal/engine/runner.go`.
3. Update the call in
   `internal/engine/bench_test.go` from
   `engine.DedupeDiagnostics` to
   `lint.DedupeDiagnostics`.
4. Update comments in
   `internal/export/export.go` and
   `internal/fix/fix.go` that reference
   `engine.ConfigureRule`,
   `engine.CheckRules`, and
   `engine.DedupeDiagnostics` to use the
   current package names.
5. Run `go test ./...` and
   `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `internal/engine/check.go` exports no
  `ConfigureRule` or `CheckRules` symbol.
- [ ] `internal/engine/runner.go` exports no
  `DedupeDiagnostics` symbol.
- [ ] No file references
  `engine.DedupeDiagnostics` in production
  or test code.
- [ ] Stale comments updated.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports
  no issues.
