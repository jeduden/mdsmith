---
id: 2610032327
title: Reuse the cached wikilink index for LSP moves and close its root
status: "🔲"
summary: >-
  Each `workspace/willRenameFiles` batch that has a `[[stem]]`
  edge walks the whole workspace root on disk, on the LSP
  request goroutine, to count same-stem files. The session's
  run cache already holds a wikilink index of that root for
  MDS027. The walk also opens an `os.Root` through
  `lint.OpenRootFS` that no caller ever closes. Reuse the
  cached index when it is fresh, and give every
  `OpenRootFS` caller a way to close its root.
model: opus
depends-on: [2610022104]
---
# Reuse the cached wikilink index for LSP moves and close its root

## Goal

An LSP file move must not walk the whole workspace tree on
the request goroutine, and no wikilink walk may leave an
`os.Root` handle open.

## Background

Plan 2610022104 made the move guard count same-stem files
against the index the wikilink resolver reads.
[rename.go](../internal/lsp/rename.go) builds that index in
`renameWorkspace` with `linkgraph.WikilinkIndexAtDir`. The
LSP request goroutine runs that walk. A large tree that the
walk does not skip (`vendor/`, `dist/`, `.venv/`) blocks
every other request until the walk ends.

The session run cache holds the index MDS027 built
(`runcache.Cache.Wikilinks`). The LSP clears that cache on
file create, delete, and rename
([server_documents.go](../internal/lsp/server_documents.go)).
But if the editor never sends those file events, the cached
index can be stale. Plan 2610022104 kept the fresh walk for
that reason.

[rootfs.go](../internal/lint/rootfs.go) `OpenRootFS` returns
`root.FS()` and drops the `*os.Root`, so the handle stays
open until garbage collection. MDS027, `mdsmith list
backlinks`, the CLI move, and the LSP move all leak it.

## Design

- Read the session's cached index when the LSP has seen a
  file event since the last walk or has file-watch
  registration. Otherwise walk fresh, as today.
- Make `OpenRootFS` return a closer (or an `fs.FS` that
  implements `io.Closer`). Every one-shot caller closes it
  after its walk. A run-cache owner closes it when the
  cache is cleared.

## Tasks

1. Write a failing LSP test: a move batch with a
   `[[stem]]` edge, a warm cached index, and file-watch
   registration does not walk the root (count walks
   through a test hook).
2. Route `renameWorkspace` to the session's cached index
   under that condition; keep the fresh walk otherwise.
3. Write a failing test: `lint.OpenRootFS` returns a
   handle that a caller can close, and a read after close
   fails.
4. Close the root in `linkgraph.WikilinkIndexAtDir`,
   backlinks, and the MDS027 run cache.
5. Run `go test ./...` and the linter.

## Acceptance Criteria

- [ ] An LSP move with a fresh cached index does not walk
      the workspace root
- [ ] An LSP move with no file-watch registration still
      walks fresh and counts a gitignored same-stem file
- [ ] No `lint.OpenRootFS` caller leaves its `os.Root` open
      after its walk ends
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
