---
id: 2610040512
title: Refused-move policy in willRenameFiles batches
status: "✅"
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

## Survey

No client tells the server. LSP 3.17 defines the
`workspace/willRenameFiles` payload as `RenameFilesParams`
`{ files: FileRename[] }`, and `FileRename` holds only
`oldUri` and `newUri`. The `overwrite` and `ignoreIfExists`
flags exist only on `RenameFileOptions`, which rides on a
`RenameFile` operation the server sends inside a
`WorkspaceEdit`. That is the opposite direction.
[`renameFilesParams`](../internal/lsp/protocol.go) models the
same two fields.

- VS Code: an Explorer rename onto an existing name fails
  with "A file or folder already exists". A drag, drop, or
  paste move onto one asks "Do you want to replace it?"
  first. The editor knows the answer, but
  `FileWillRenameEvent.files` carries only `oldUri` and
  `newUri`. `vscode-languageclient` forwards those two, so
  the server cannot tell an overwrite from a decline.
- Neovim: core `vim.lsp.util.rename` sends no
  `willRenameFiles` at all. It skips an existing target
  unless `overwrite` is set. Plugins such as
  `nvim-lsp-file-operations` and `oil.nvim` send the request,
  and they use the same two-field payload.
- Obsidian: `Vault.rename` and `FileManager.renameFile` throw
  "Destination file already exists" for an existing path. The
  mdsmith plugin runs the WebAssembly engine, not `mdsmith
  lsp`. It plans a move only on an explicit request, and it
  plans it with `move`, which refuses an existing
  destination outright.

Policy: withhold and count (task 3). A link to or from a
refused move gets no edit unless one spelling is right
whether the host moves the file or not. The warning counts
every such link that may reach a different file. Otherwise
the link stops resolving, where MDS027 flags it.

## Tasks

1. [x] Survey how VS Code, Neovim, and Obsidian behave when a
   `workspace/willRenameFiles` rename targets an existing
   file: overwrite, prompt, or fail. Record whether the
   request carries anything (such as `ignoreIfExists` or
   `overwrite`) that tells the server. See Survey.
2. [x] If the client tells the server, spell links to and from a
   refused move as if the move runs when the client will
   overwrite. Keep the withhold-and-count policy otherwise.
   Not applicable: no client tells the server (see Survey).
3. [x] If no client tells it, record the withhold-and-count
   policy in [`docs/reference/cli/move.md`](../docs/reference/cli/move.md)
   as the settled behavior and close the plan. Recorded in
   that page's `mdsmith lsp` entry under See also. The page
   is at its file-length budget, so the entry was reworded in
   place, not given a section of its own.
4. [x] Unit tests in
   [`moveall_test.go`](../internal/refactor/moveall_test.go)
   for each refused-move case under the chosen policy. The
   cases settled by plan 2610030438 were already covered.
   The survey of cases found one gap: a link inside a refused
   move that leaves its folder, to a file the batch does not
   plan to move, got no count. If the host moves the file
   anyway, the link reads from the new folder and can reach
   another existing file silently.
   `TestMoveAll_RefusedHolderMisreads` covers it, and
   `countMisread` now counts it. A link to the file itself
   is not counted, and one to a path a refused move may
   overwrite is. A file the workspace does not list, such
   as an image, is read to see whether it is there.
   `countRefusedHolders` reads each such member from the
   text the batch already holds, so a refused lone move
   lists no file. A link in such a member to a planned
   member whose old path another member takes is counted
   too (`TestMoveAll_RefusedHolderLeftInPlace`): left in
   place, the file's link reaches the newcomer.
   A link from any file to the existing file a refused move
   lands on is counted too, by path or by wikilink
   (`TestMoveAll_OverwrittenDestinationReferrers`): if the
   host overwrites it, the link reaches the moved file. An
   earlier draft left this case uncounted, which broke the
   second acceptance criterion. A planned member's link to
   that file is still re-spelled. `Move` now validates its
   one pair first, so a refused lone move reads no file.

## Acceptance Criteria

- [x] A link touching a refused move either gets the spelling
      the client's rename outcome makes right, or is left to
      stop resolving and is counted in the warning. Read as
      "fails loudly": a link that may reach a different file
      is counted, and one that only stops resolving is left
      to MDS027, as plan 2610030438 settled.
- [x] No link touching a refused move silently resolves to a
      different file than it did before the batch.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
