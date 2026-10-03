---
id: 2610030438
title: Batch-aware planning for multi-file willRenameFiles
status: "🔲"
summary: >-
  When one workspace/willRenameFiles request moves several Markdown
  files that link to each other, plan every link rewrite against the
  post-batch locations of both ends, so a link between two moved
  files gets the one correct edit instead of being withheld.
model: opus
depends-on: []
---
# Batch-aware planning for multi-file willRenameFiles

## Goal

Some links join two files that move in the same
`workspace/willRenameFiles` request. Give each such link its
one correct rewrite. Do not just withhold it.

## Background

[`handleWillRenameFiles`](../internal/lsp/fileops.go) runs one
`refactor.Move` per renamed file. Each runs against the same
pre-batch index snapshot. Take `a.md` with `[b](b.md)`, renamed
with `b.md` to `x/a.md` and `x/b.md`. The move of `a.md`
rewrites the link to `../b.md`, and the move of `b.md` rewrites
it to `x/b.md`. Both are wrong: the right text is `b.md`.

Code review of PR #889 found this. That PR added a stopgap,
`dropConflictingTextEdits`. It withholds any pair of edits whose
ranges overlap, so the client gets a valid WorkspaceEdit. The
withheld link stays stale whenever the two files land in
different folders, and MDS027 then reports it.

## Tasks

1. Add a batch entry point in
   [`internal/refactor`](../internal/refactor/move.go), such as
   `MoveAll(ws, []MovePair)`. It resolves each link target
   against the batch rename map, so a link from one moved file
   to another is spelled from the source's new folder to the
   target's new path.
2. Merge the per-file plans into one `Plan`. Each range gets
   one edit, and the incoming-link and outbound-link passes no
   longer both rewrite a link between two moved files.
3. Switch `handleWillRenameFiles` to the batch entry point.
   Keep `dropConflictingTextEdits` only as a guard, with a
   test proving it no longer fires for this case.
4. Unit tests: two moved files linking each other in the same
   new folder and in different new folders, wikilinks between
   moved files, and a three-file cycle.

## Acceptance Criteria

- [ ] Moving `a.md` and `b.md` into `x/` in one request leaves
      `[b](b.md)` unchanged and returns no edit for that range.
- [ ] Moving `a.md` to `x/a.md` and `b.md` to `y/b.md` in one
      request rewrites the link to `../y/b.md`.
- [ ] No reply holds two edits with overlapping ranges in one
      file.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
