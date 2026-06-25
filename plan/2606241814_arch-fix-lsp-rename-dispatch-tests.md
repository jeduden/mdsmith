---
id: 2606241814
title: >-
  Add unit tests for lsp/rename dispatch helpers
  and workspace adapter methods
status: "✅"
model: sonnet
summary: >-
  Add TestServer_PrepareRenameAt,
  TestServer_RenameHeading, TestServer_RenameLinkRef,
  TestLspRenameWorkspace_Resolve, and "// no test by
  design" exemption comments for two trivial
  lspRenameWorkspace one-liners in
  internal/lsp/rename.go.
---
# Add unit tests for lsp/rename dispatch helpers

## Context

Audit 2026-06-24 (range: 09f22d3..3d35b77)
flagged test debt in
[internal/lsp/rename.go](../internal/lsp/rename.go).

Plans 2606240212 and 2606240214 closed the gaps.
Four functions have no dedicated tests. Three are
`*Server` dispatch helpers; one is an adapter:

- `(s *Server) prepareRenameAt` — dispatches on the
  locate result to call `headingPrepareRange`,
  `refDefPrepareRange`, or `refUsePrepareRange`.
- `(s *Server) renameHeading` — builds an
  `lspRenameWorkspace`, calls `rename.Heading`,
  converts the result to LSP changes.
- `(s *Server) renameLinkRef` — calls
  `rename.LinkRef`, converts the result to LSP
  changes.
- `lspRenameWorkspace.Resolve` — looks up a file in
  the open-document buffer or falls back to disk.
- `lspRenameWorkspace.IncomingAnchorEdges` and
  `.Files` — trivial one-liner delegations that need
  "// no test by design" exemption comments.

## Goal

Every function in `internal/lsp/rename.go` must
have a named unit test. Trivial one-liners need
"// no test by design" instead. See Tests doc
§"every function has a dedicated unit test"
and §"Exemptions".

## Tasks

1. Add `TestServer_PrepareRenameAt` in a new or
   existing `*_test.go` in `internal/lsp/`. Drive
   it with an in-memory server and synthetic source;
   cover the heading, refDef, refUse, and prose-only
   cases.
2. Add `TestServer_RenameHeading` covering the
   happy path (workspace with one cross-file anchor
   edge) and the collision / invalid-slug error paths.
3. Add `TestServer_RenameLinkRef` covering the happy
   path and the empty-label / collision error paths.
4. Add `TestLspRenameWorkspace_Resolve` covering the
   open-document (buffer) path and the disk-fallback
   path. Use `safeBuffer` or similar for the writer.
5. Add "// no test by design" comments to
   `lspRenameWorkspace.IncomingAnchorEdges` and
   `lspRenameWorkspace.Files` in `rename.go`.
6. Run `go test ./internal/lsp/...` — green.
7. Run `mdsmith check .` — 0 failures.

## Acceptance Criteria

- [x] `TestServer_PrepareRenameAt` exists and passes.
- [x] `TestServer_RenameHeading` exists and passes.
- [x] `TestServer_RenameLinkRef` exists and passes.
- [x] `TestLspRenameWorkspace_Resolve` exists and
      passes.
- [x] `IncomingAnchorEdges` and `Files` carry
      "// no test by design" comments.
- [x] `go test ./internal/lsp/...` — green.
- [x] `go test ./...` — green.
- [x] `mdsmith check .` — 0 failures.
