---
id: 2606162213
title: "Document internal/checker and internal/rulelayer; remove engine shim"
status: "✅"
model: sonnet
depends-on: []
summary: >-
  Two new production packages from the
  lazy-parse series are absent from the
  architecture docs, and an unexported
  forwarding shim survives in
  internal/engine/check.go.
---
# Document new packages; remove engine shim

## Context

Closes two entries from the
[2026-06-16 audit][audit]:

- `internal/checker` and
  `internal/rulelayer` absent from
  architecture docs.
- `internal/engine/check.go` residual shim.

[audit]: ../docs/development/architecture-audit.md

## Goal

Add both packages to the architecture docs.
Delete the `check.go` forwarding shim.

## Tasks

1. In `docs/development/architecture/go.md`,
   add to the SRP table:

   ```markdown
   - `internal/checker` — shared
     rule-checking primitives (classify,
     dispatch, filter) used by both
     `internal/engine` and
     `internal/fix`.
   - `internal/rulelayer` — maps each
     rule ID to its lazy-parse layer
     (Layer 0 vs. AST-required) from the
     embedded rule-walk audit manifest.
   ```

2. In `docs/development/architecture/index.md`,
   add both packages to the layering diagram
   under `internal/engine`'s dependency list.

3. In `internal/engine/runner.go`, find
   the call to `checkRulesWithIntraFile`.
   Replace it with a direct call to
   `checker.CheckRulesWithIntraFile`.
   Add the `internal/checker` import.

4. Delete `internal/engine/check.go`.

5. Run `go build ./...` and `go test ./...`
   to confirm no breakage.

6. Run `go run ./cmd/mdsmith fix .` from
   the workspace root to refresh catalogs.

## Acceptance Criteria

- [x] `go.md` SRP table includes entries
      for both `internal/checker` and
      `internal/rulelayer`.
- [x] `index.md` layering diagram lists
      both packages.
- [x] `internal/engine/check.go` does not
      exist.
- [x] `runner.go` calls
      `checker.CheckRulesWithIntraFile`
      directly (no intermediary function).
- [x] `go test ./...` passes.
- [x] `go build ./...` succeeds.
- [x] `go run ./cmd/mdsmith check .`
      reports 0 failures.
