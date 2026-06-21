---
id: 2606211909
title: 'arch-fix: split internal/lsp/server.go'
status: '🔲'
summary: >-
  server.go crept back to 1 007 lines after
  plan 203 green. Split along dispatch groups
  to stay under the 1 000-line threshold named
  in the checklist.
model: ''
depends-on: []
---
# arch-fix: split internal/lsp/server.go

## Context

Audit 2026-06-21 flagged
[`internal/lsp/server.go`](../internal/lsp/server.go)
at 1 007 lines.

Plan 203 (`arch-fix-lsp-server-split`) was
green. The VS Code `kinds` and `rule-doc`
capability wiring added in the security and
editor-integration series grew the file back
over the threshold.

The audit checklist names `server.go`
explicitly as a file to split by dispatch
group when it exceeds 1 000 lines.

## Goal

Split `server.go` into sibling files within
`internal/lsp` by dispatch group.
The primary file stays under 800 lines.

## Tasks

1. Identify the new capability dispatch
   blocks added since plan 203 (`kinds`
   handlers, `rule-doc` / hover-rewrite
   handlers).
2. Create `internal/lsp/server_kinds.go`:
   move the `kinds` request handlers and
   their helpers.
3. Move hover-rewrite and rule-doc
   registration into a dedicated sibling
   (or extend the existing one from
   plan 203).
4. Remove the moved blocks from `server.go`
   and trim unused imports.
5. Run `go build ./...` — confirm no errors.
6. Run `go test ./...` — confirm no
   regressions.
7. Confirm `wc -l server.go` is under
   800 lines.

## Acceptance Criteria

- [ ] `internal/lsp/server.go` is under
  800 lines.
- [ ] `go build ./...` passes.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` reports
  no new issues.
- [ ] No LSP behaviour changed — pure file
  reorganisation within the `lsp` package.
