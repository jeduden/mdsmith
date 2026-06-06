---
id: 203
title: Split internal/lsp/server.go and symbols.go
status: "✅"
summary: >-
  server.go (1 536 lines) and symbols.go (1 385
  lines) both exceed the 1 000-line threshold.
  Split each file along its natural dispatch
  group boundaries.
model: ""
depends-on: []
---
# Split internal/lsp/server.go and symbols.go

## Goal

[internal/lsp/server.go](../internal/lsp/server.go) is 1 536 lines.
[internal/lsp/symbols.go](../internal/lsp/symbols.go) is 1 385 lines.
Both exceed the 1 000-line ceiling from
the audit checklist. Both contain LSP
capability handlers that can live in
their own files. The pattern already
exists: [rename.go](../internal/lsp/rename.go) and
[completion.go](../internal/lsp/completion.go)
each own a dispatch group.

## Tasks

1. Identify the dispatch groups in
   `server.go` (lifecycle, doc-sync,
   code-action, diagnostics push).
2. Move each group to a new file
   (`server_lifecycle.go`, etc.).
3. Identify the dispatch groups in
   `symbols.go` (document symbols,
   workspace symbols, call hierarchy).
4. Move each group to a new file
   (`symbols_document.go`, etc.).
5. Verify no file in `internal/lsp/`
   exceeds 1 000 lines.
6. Run `go build ./...` and
   `go test ./...`.

## Acceptance Criteria

- [x] No non-test production file under
  `internal/lsp/` exceeds 1 000 lines
  (`*_test.go` files are out of scope).
- [x] `go build ./...` clean.
- [x] `go test ./...` passes (LSP
  integration tests included).
- [x] `go tool golangci-lint run` clean.
