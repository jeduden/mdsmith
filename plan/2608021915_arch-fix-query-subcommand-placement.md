---
id: 2608021915
title: >-
  Move list-query subcommand logic out of
  cmd/mdsmith/main.go into query.go
status: "🔲"
model: haiku
summary: >-
  parseQueryFlags, runQuery, queryFiles, and
  readFrontMatterRaw implement the `mdsmith list
  query` subcommand but live in main.go instead of a
  dedicated per-subcommand file. Flagged by the
  2026-08-02 audit.
---
# Move list-query subcommand logic out of cmd/mdsmith/main.go into query.go

## Goal

Relocate the `list query` subcommand's implementation
into its own file. That keeps `main.go` as
dispatch-and-glue, matching the pattern already used
for every sibling subcommand.

## Background

The 2026-08-02 audit (see
[the audit log](../docs/development/architecture-audit.md))
found this placement gap:

- [main.go](../cmd/mdsmith/main.go) defines
  `parseQueryFlags`, `runQuery`, `queryFiles`, and
  `readFrontMatterRaw` — the full domain logic for the
  `list query` subcommand (CUE-expression matching
  against front matter, its own file walk).
- [list.go](../cmd/mdsmith/list.go)'s `runList`
  dispatches `case "query": return runQuery(...)`, the
  same way it dispatches `case "backlinks":` to
  `runBacklinks`, which lives in its own
  [backlinks.go](../cmd/mdsmith/backlinks.go).
- [go.md](../docs/development/architecture/go.md)'s
  "Clean wiring in `cmd/mdsmith`" section: domain logic
  belongs in `pkg/mdsmith`, `internal/engine`, or a
  subcommand's own file — not in `main.go`.
- The 2026-07-19 audit already applied this exact move
  once, relocating the `init` subcommand's logic out of
  `main.go` into
  [init.go](../cmd/mdsmith/init.go); this plan repeats
  that move for `query`.

## Tasks

1. Create `cmd/mdsmith/query.go` and move
   `parseQueryFlags`, `runQuery`, `queryFiles`, and
   `readFrontMatterRaw` into it, unchanged.
2. Move the query-only tests covering those functions
   out of `main_unit_test.go` into a new
   `cmd/mdsmith/query_unit_test.go`, following the
   `init_unit_test.go` precedent.
3. Confirm `list.go`'s `runList` still compiles against
   the relocated `runQuery` with no signature change.
4. `go build ./...` passes.
5. `go test ./cmd/mdsmith/...` passes.
6. `go tool -modfile=tools/go.mod golangci-lint run`
   reports no issues.

## Acceptance Criteria

- [ ] `parseQueryFlags`, `runQuery`, `queryFiles`, and
      `readFrontMatterRaw` no longer appear in
      `cmd/mdsmith/main.go`.
- [ ] `cmd/mdsmith/query.go` holds the relocated code
      with no behavior change.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
