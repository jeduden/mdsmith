---
id: 2606240214
title: >-
  Remove duplicated helpers between lsp/rename.go
  and rename/rename.go
status: "🔲"
model: sonnet
summary: >-
  normalizedLabel and refDefBracketBytes are
  duplicated with identical bodies in two packages.
  Export them from rename and remove the lsp copies.
  Flagged by the 2026-06-24 audit.
---
# Remove duplicated rename helpers

## Goal

Export `normalizedLabel` and `refDefBracketBytes` from
`internal/rename` and delete the copies in
`internal/lsp/rename.go`.

## Background

The 2026-06-24 audit (range: 1599c9f..09f22d3) found
that two private helpers are duplicated with identical
bodies:

- `normalizedLabel(b []byte) string` — wraps
  `util.ToLinkReference(b)`
- `refDefBracketBytes(row []byte) []int` — parses
  bracket bounds on a `[label]:` line

`internal/lsp` already imports `internal/rename`.
Hub §"Anti-patterns we have actually hit" flags this
kind of silent copy as maintenance friction.
If either copy is updated, the other diverges without
a compile error.

## Tasks

1. In `internal/rename/rename.go`, rename
   `normalizedLabel` → `NormalizedLabel` and
   `refDefBracketBytes` → `RefDefBracketBytes`.
2. Update all callers in `internal/rename/` to use
   the exported names.
3. Add or update tests in
   `internal/rename/rename_test.go` for both.
4. In `internal/lsp/rename.go`, delete the private
   copies of both functions.
5. Replace every call site in `lsp/rename.go` with
   `rename.NormalizedLabel` and
   `rename.RefDefBracketBytes`.
6. `go build ./...` passes.
7. `go test ./internal/lsp/... ./internal/rename/...`
   passes.

## Acceptance Criteria

- [ ] `internal/lsp/rename.go` has no private
      `normalizedLabel` or `refDefBracketBytes`.
- [ ] `internal/rename/rename.go` exports
      `NormalizedLabel` and `RefDefBracketBytes`.
- [ ] `internal/rename/rename_test.go` has dedicated
      tests for both helpers.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
