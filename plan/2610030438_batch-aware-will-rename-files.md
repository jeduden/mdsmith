---
id: 2610030438
title: Batch-aware planning for multi-file willRenameFiles
status: "✅"
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
ranges overlap, so the client gets a valid WorkspaceEdit. A
second guard, `dropCrossMoveEdits`, withholds any path rewrite
one move plans inside another moved file whose folder changes.
It keeps a `[[stem]]` rewrite, which no folder change affects. A
`window/logMessage` warning gives the withheld count.

The withheld link stays stale whenever the batch changes the
relative path between the two files, such as `b.md` moving to
`x/c.md` or the two files landing in different folders. MDS027
then reports it.

## Tasks

1. [x] Add a batch entry point in
   [`internal/refactor`](../internal/refactor/move.go), such as
   `MoveAll(ws, []MovePair)`. It resolves each link target
   against the batch rename map, so a link from one moved file
   to another is spelled from the source's new folder to the
   target's new path.
2. [x] Merge the per-file plans into one `Plan`. Each range gets
   one edit, and the incoming-link and outbound-link passes no
   longer both rewrite a link between two moved files.
3. [x] Switch `handleWillRenameFiles` to the batch entry point.
   Keep `dropConflictingTextEdits` only as a guard, with tests
   proving it no longer fires for these cases. Remove
   `dropCrossMoveEdits`: the batch plans no path edit inside a
   member that changes folder except its own outbound ones, so
   no input can drive it.
4. [x] Unit tests: two moved files linking each other in the same
   new folder and in different new folders, wikilinks between
   moved files, and a three-file cycle.
5. [x] Cover the case only one move rewrites. `docs/a.md` links
   `../docs/b.md`, and one request moves it to `other/a.md` and
   `docs/b.md` to `docs/sub/b.md`. The token still resolves from
   `other/`, so the move of `a.md` emits no edit. The move of
   `b.md` emits `sub/b.md`, spelled from `docs/`. The stopgap
   withholds it, but the right text is `../docs/sub/b.md`.
6. [x] Keep a one-sided rewrite that is already right. `docs/a.md`
   links `../b.md`, and one request moves it to `other/a.md`
   and `b.md` to `b2.md`. The move of `b.md` emits `../b2.md`,
   which also resolves from `other/`, yet the stopgap withholds
   it.
7. [x] Plan a chain such as `b.md` to `z.md` plus `a.md` to `b.md`.
   `refactor.Move` refuses `a.md` because `b.md` exists in the
   pre-batch snapshot, so links to `a.md` stay stale and the
   warning does not count them.
8. [x] Guard `[[stem]]` rewrites against a stem two moves share.
   One request moves `x/a.md` to `x/c.md` and `y/b.md` to
   `y/c.md`. Each move checks the new stem against the
   pre-batch snapshot, finds no `c`, and rewrites its links to
   `[[c]]`. The stopgap keeps both, so after the batch every
   such link names two files. The same holds when the file
   `[[Guide]]` reaches and a same-stem sibling both move to one
   new stem: `docs/Guide.md` to `z/Manual.md` and `ref/Guide.md`
   to `a/Manual.md`. The first move rewrites every `[[Guide]]`
   to `[[Manual]]`, which after the batch reaches
   `a/Manual.md`, the former sibling.
9. [x] Rewrite folder-prefixed `[[stem]]` links when two moves
   leave a shared stem. One request moves `x/guide.md` to
   `x/manual.md` and `y/guide.md` to `y/howto.md`. The move of
   `x/guide.md`, which `[[guide]]` reaches, rewrites those links
   to `[[manual]]`, but leaves `[[y/guide]]`, which names the
   sibling's folder. The move of `y/guide.md` does not win the
   pre-batch stem and rewrites nothing, so `[[y/guide]]`
   dangles. No edit is withheld, so no warning names it.
10. [x] Warn only about a link that no longer resolves. Moving
    `docs/a.md` and `docs/b.md` into `docs/sub/` withholds both
    rewrites of each link between them, and the kept text is
    right. The warning still says "withheld 4 link rewrite(s)"
    for those two links: it counts edits, not links.
11. [x] Count a rewrite that assumes an unplanned move stayed put.
    One request moves `docs/a.md` to `other/a.md` and
    `docs/b.md` onto an existing `x/b.md`. The move of `a.md`
    spells its `b.md` link as `../docs/b.md`, a path the batch
    empties. The edit is kept and the warning stays silent.
12. [x] Count a link a chain shadows. One request moves `b.md`
    onto an existing `c.md`, which is refused, and `a.md` to
    `b.md`. Every link to the old `b.md` then reaches the
    former `a.md` and still resolves, so MDS027 cannot flag
    it. The warning counts each such link.
13. [x] Keep the batch's cost proportional to the batch. Scan
    the workspace for incoming links once, not once per
    planned move. Read each moved file once. Build the
    post-batch wikilink index as an overlay on the keys the
    moves touch.
14. [x] Count a shadowed file's links to itself alike. In
    the chain of task 12, `[self](b.md)` inside `b.md` and
    `[[b]]` both reach the newcomer once the host moves
    `b.md`, so both are counted. A path link that still
    names the file from where it lands is not.
15. [x] Refuse a `[[stem]]` rewrite onto a destination an
    indexed file spells in another letter case alone, as the
    lone-move check did before the post-batch index held the
    destination.
16. [x] Remove `dropCrossMoveEdits` and `BatchPlan.Own` and
    `StemEdits`, which only fed it. No input drives it.
17. [x] Plan one edit per `[[stem]]` link when the wikilink
    index lacks two same-stem sources, as a walk that skips a
    symlinked folder leaves them out. Only the source that
    sorts first among them wins the stem.
18. [x] Refuse two pairs whose destinations differ in letter
    case alone, such as `docs/c.md` and `Docs/C.md`. A
    case-insensitive file system stores both as one file.
19. [x] Read a file once across a batch's `[[stem]]` passes.
    A file linking three moved files is read once by them all,
    not once per move.
20. [x] Say that `Withheld` counts path and `[[stem]]` links to
    a shadowed file, not a typed `[[name.ext]]` link. The
    index has no edge lookup for a typed link. Plan
    [2610040606](2610040606_move-typed-wikilinks.md) adds it.

## Acceptance Criteria

- [x] Moving `a.md` and `b.md` into `x/` in one request leaves
      `[b](b.md)` unchanged and returns no edit for that range.
- [x] Moving `a.md` to `x/a.md` and `b.md` to `y/b.md` in one
      request rewrites the link to `../y/b.md`.
- [x] No reply holds two edits with overlapping ranges in one
      file.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues

## Follow-ups

`PLAN.md` holds one row per plan and sits at the 300-line
cap that MDS022 sets. The next plan file will fail the
check. Two fixes need the maintainer: raise the cap for
`PLAN.md` in `.mdsmith.yml`, or list only open plans in
`PLAN.md` and move completed rows to an archive catalog.
The `pick-plan` skill reads `PLAN.md`, so it must follow
the split. File this as its own plan once `PLAN.md` has
room. Round 3 of the review freed one line by dropping the
blank line between the catalog marker and its table, and
used it for plan 2610040606, so `PLAN.md` is at 300 again.
