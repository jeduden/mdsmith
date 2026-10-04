---
id: 2610040512
title: Refused-move policy in willRenameFiles batches
status: "🔲"
summary: >-
  Decide how a workspace/willRenameFiles batch spells a link to or
  from a file whose own move refactor.MoveAll refused, given that the
  editor may or may not perform that rename, and make the planner and
  the warning follow that one policy.
model: opus
depends-on: [2610030438]
---
# Refused-move policy in willRenameFiles batches

## Goal

A link that touches a refused move should get the spelling
that is right for what the editor really does with that file.
When that is not known, the link should fail where MDS027
can see it.

## Background

[`refactor.MoveAll`](../internal/refactor/moveall.go) refuses
a pair such as `docs/b.md` to an existing `x/b.md`. The batch
still treats the file as a member, because an editor such as
VS Code applies the `workspace/willRenameFiles` edits and
then attempts every rename. Whether a rename onto an existing
file runs depends on the client: it may overwrite, prompt, or
fail.

Plan 2610030438 settled these cases:

- A link from a planned holder to a refused file gets no edit.
  The warning counts it, even when it reads as the refused
  destination from the holder's new folder, and MDS027 flags
  it when it stops resolving.
- A link inside a refused file gets no edit, unless that file
  stays in its own folder.
- A link to a refused file whose old path another planned
  member takes is counted in the warning (`countShadowed`).
  Without the count, the link would silently reach the
  newcomer. The refused file's own links to itself count the
  same way, unless a path link still names the file from
  where it lands.

Code review of PR #907 asked whether a planned holder should
instead spell such a link to the refused file's destination.
That is right if the editor overwrites, but it silently
reaches the old `x/b.md` if the editor declines. No spelling
is right both ways. The current choice fails loudly. This plan
decides whether a client signal can settle the case.

## Tasks

1. Survey how VS Code, Neovim, and Obsidian behave when a
   `workspace/willRenameFiles` rename targets an existing
   file: overwrite, prompt, or fail. Record whether the
   request carries anything (such as `ignoreIfExists` or
   `overwrite`) that tells the server.
2. If the client tells the server, spell links to and from a
   refused move as if the move runs when the client will
   overwrite. Keep the withhold-and-count policy otherwise.
3. If no client tells it, record the withhold-and-count
   policy in [`docs/reference/cli/move.md`](../docs/reference/cli/move.md)
   as the settled behavior and close the plan.
4. Unit tests in
   [`moveall_test.go`](../internal/refactor/moveall_test.go)
   for each refused-move case under the chosen policy.

## Acceptance Criteria

- [ ] A link touching a refused move either gets the spelling
      the client's rename outcome makes right, or is left to
      stop resolving and is counted in the warning.
- [ ] No link touching a refused move silently resolves to a
      different file than it did before the batch.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
