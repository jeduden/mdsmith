---
id: 195
title: Split cmd/mdsmith/main.go into per-subcommand files
status: "🔲"
summary: >-
  main.go is 1 306 lines with six handlers over
  50 lines. Move each oversized handler to its own
  per-subcommand file, following the pattern used
  by kinds.go, metrics.go, and backlinks.go.
model: ""
depends-on: []
---
# Split cmd/mdsmith/main.go into per-subcommand files

## Goal

[cmd/mdsmith/main.go](../cmd/mdsmith/main.go) is 1 306 lines.
Six handlers exceed ~50 lines: `runHelp`
(81), `runFix` (71), `fixDiscovered`
(68), `runCheck` (62), `checkStdin`
(61), `run` (57). The per-subcommand
file pattern is already used by
[kinds.go](../cmd/mdsmith/kinds.go),
[metrics.go](../cmd/mdsmith/metrics.go),
[backlinks.go](../cmd/mdsmith/backlinks.go),
and [mergedriver.go](../cmd/mdsmith/mergedriver.go).

## Tasks

1. Create `cmd/mdsmith/check.go`; move
   `runCheck`, `checkStdin`, and helpers.
2. Create `cmd/mdsmith/fix.go`; move
   `runFix`, `fixDiscovered`, and helpers.
3. Create `cmd/mdsmith/help.go`; move
   `runHelp` and helpers.
4. Remove the moved functions from
   `main.go`; verify it drops below
   700 lines.
5. Run `go build ./...` and
   `go test ./...`.

## Acceptance Criteria

- [ ] `cmd/mdsmith/main.go` is under
  700 lines.
- [ ] No handler body exceeds ~50 lines.
- [ ] `check.go`, `fix.go`, `help.go`
  exist under `cmd/mdsmith/`.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` clean.
