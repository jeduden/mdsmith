---
id: 2610032327
title: Reuse the cached wikilink index for LSP moves and close its root
status: "✅"
summary: >-
  Each `workspace/willRenameFiles` batch that has a `[[stem]]`
  edge walks the whole workspace root on disk, on the LSP
  request goroutine, to count same-stem files. The session's
  run cache already holds a wikilink index of that root for
  MDS027. The walk also opens an `os.Root` through
  `lint.OpenRootFS` that no caller ever closes. Reuse the
  cached index when it is fresh, and give every
  `OpenRootFS` caller a way to close its root. Also give the
  move its own seam, so a heading rename no longer carries a
  wikilink index, and build `Session.Move`'s two indexes from
  one walk.
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

The PR #904 pre-merge review found three more costs on the
same seam. [heading.go](../internal/refactor/heading.go)
`Workspace` makes every heading-rename surface supply
`WikilinkIndex`, which only a move reads. So `renameHeading`
takes a config snapshot and builds a closure it never calls.
`lspRenameWorkspace.WikilinkIndex` calls its `wikilinks`
field with no nil check, so a literal workspace panics once a
move reaches a `[[stem]]` edge.
[refactor.go](../pkg/mdsmith/refactor.go)
`sessionRefactorWorkspace` walks its FS snapshot twice: once
for the edge index and once for `WikilinkIndex`.

## Design

- Read the session's cached index once the client accepts
  the `**/*` file-watch registration. A watched-file event
  alone proves nothing: a client may sync a narrower glob.
  Otherwise walk fresh, as today.
- Make `OpenRootFS` return a closer (or an `fs.FS` that
  implements `io.Closer`). Every one-shot caller closes it
  after its walk. A run-cache owner closes it when the
  cache is cleared.

## Tasks

1. [x] Write a failing LSP test: a move batch with a
   `[[stem]]` edge, a warm cached index, and file-watch
   registration does not walk the root (count walks
   through a test hook).
2. [x] Route the move workspace (`moveWorkspace`, split from
   `renameWorkspace` in task 6) to the session's cached index
   under that condition, only when the session was built at
   the move's root; keep the fresh walk otherwise.
3. [x] Write a failing test: `lint.OpenRootFS` returns a
   handle that a caller can close, and a read after close
   fails.
4. [x] Close the root in `linkgraph.WikilinkIndexAtDir`,
   backlinks, and the MDS027 run cache.
5. [x] Write a failing compile-level test: a heading-rename
   stub with no `WikilinkIndex` method satisfies the seam
   `refactor.Heading` takes. Split `Workspace` into that seam
   and a move seam that adds the path, wikilink-edge, and
   `WikilinkIndex` questions.
6. [x] Build the LSP heading workspace without the wikilink
   closure, and the move workspace only through a
   constructor that sets it.
7. [x] Write a failing `Session.Move` test that counts FS walks:
   a move with a `[[stem]]` edge walks once. Collect the
   walked paths in the edge-index walk and build the index
   with `linkgraph.NewWikilinkIndexFromPaths`.
8. [x] Run `go test ./...` and the linter.
9. [x] Write failing tests that record every root
   `lint.OpenRootFS` opens (one `rootfstest.Record` hook) and
   close each at its owner: `lintFile` for an on-disk lint,
   `RunSource` for a call no parse cache keeps, the `Session`
   (in `Dispose`) for the root it lends its runners, and the
   fix, export, and extract calls for their file.
10. [x] Write failing tests that a config reload closes the
    superseded LSP session's lent root and its overlay's disk
    root once the last lint holding it returns. Hand out a
    release from `currentSession` and `sessionAt` (a
    `sessionLease` holder count), and add
    `OverlayWorkspace.Close`.

## Acceptance Criteria

- [x] An LSP move with a fresh cached index does not walk
      the workspace root
- [x] An LSP move with no file-watch registration still
      walks fresh and counts a gitignored same-stem file
- [x] No wikilink walk leaves its `os.Root` open after the
      walk ends, and the per-file lint, fix, export and
      extract roots close when their file is released; a
      superseded LSP session's roots close once no lint
      holds it
- [x] `refactor.Heading` accepts a workspace with no
      `WikilinkIndex` method, and an LSP heading rename builds
      no wikilink closure
- [x] No LSP move workspace can hold a nil wikilink closure
- [x] `Session.Move` with a `[[stem]]` edge walks the
      workspace FS once
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues

## Follow-up

These items need their own plan; PLAN.md sits at its
300-line file-length limit, so filing it needs a maintainer
decision on the limit first.

- [server_lifecycle.go](../internal/lsp/server_lifecycle.go):
  an accepted `**/*` registration can still miss events
  (`files.watcherExclude`, exhausted inotify watches), so a
  move may read a stale wikilink index. A bound on how long
  the cache is trusted, or a cheap freshness probe, needs a
  design decision.
- [server_documents.go](../internal/lsp/server_documents.go):
  every create or delete outside `.git` and `node_modules`
  drops the wikilink index and catalog glob caches, since the
  index keys every file. A build writing many files (`dist/`)
  makes the lints in between walk the tree again. Narrowing
  needs the index to skip ignored directories, which changes
  what MDS027 resolves.
