---
id: 2607191918
title: >-
  Deduplicate isClaimed between internal/schema and
  requiredstructure
status: "🔲"
model: haiku
summary: >-
  internal/rules/requiredstructure/rule.go carries a
  byte-for-byte copy of internal/schema/validate.go's
  isClaimed helper, despite already importing
  internal/schema. Flagged by the 2026-07-19 audit.
---
# Deduplicate isClaimed between internal/schema and requiredstructure

## Goal

Remove the duplicated `isClaimed` helper so the claimed-range
check has one implementation.

## Background

The 2026-07-19 audit (see
[the audit log](../docs/development/architecture-audit.md))
found this duplication:

- [requiredstructure/rule.go](../internal/rules/requiredstructure/rule.go)
  carries a copy of
  [schema/validate.go](../internal/schema/validate.go)'s
  `isClaimed` helper.
- Its comment reads "Mirrors
  internal/schema/validate.go's isClaimed for the
  identical pattern" instead of reusing it.
- `requiredstructure/rule.go` already imports
  `internal/schema`. This isn't an import-cycle
  workaround — it's a plain copy.
- [go.md](../docs/development/architecture/go.md)'s
  refactor-moves section names this exact shape: "Lift a
  shared dependency up to an interface... once two rules
  needed the same shape."

## Tasks

1. Export the helper from `internal/schema` (e.g.
   `schema.IsClaimed`) or move it to a small shared
   set-helper both packages can import without creating a
   cycle — check `internal/schema`'s existing dependency
   direction against `requiredstructure` first, since
   `requiredstructure` importing `schema` already establishes
   the direction that must hold.
2. Delete `requiredstructure/rule.go`'s private copy and call
   the shared helper instead.
3. Keep or adapt the existing test coverage for both call
   sites; do not remove test coverage in the move.
4. `go build ./...` passes.
5. `go test ./internal/schema/... ./internal/rules/requiredstructure/...`
   passes.

## Acceptance Criteria

- [ ] Only one implementation of the claimed-range check
      exists in the repository.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
