---
id: 2610022044
title: Count only Markdown files as wikilink stem siblings on move
status: "✅"
summary: >-
  The move planner's wikilink pass treats a non-Markdown file
  (for example an extensionless `LICENSE`) as a stem target,
  though the wikilink index only maps `.md`/`.markdown` files
  to stems. A listed `LICENSE` blocks the `[[license]]`
  rewrite when `docs/license.md` moves, and moving `LICENSE`
  itself rewrites `[[license]]` links that point at
  `docs/license.md`. Only Markdown files may count or move as
  stem targets. A destination with a non-Markdown extension
  keeps an exact-name collision check.
model: sonnet
depends-on: []
---
# Count only Markdown files as wikilink stem siblings on move

## Goal

On a move, treat only `.md` and `.markdown` files (any case)
as wikilink stem targets. Those are the only files the
wikilink index maps to a stem. This holds for the same-stem
sibling count and for the moved file itself.

## Background

[move.go](../internal/refactor/move.go) has `fileStem`. It
calls `linkgraph.WikilinkStem` on the file's basename. That
helper reads a link target, not a file: an empty extension is
a bare page, so `WikilinkStem("LICENSE")` returns `license`.
The fallback to the whole basename never runs for such a file.
So `fileStem("notes/LICENSE")` is `license`, the same key as
`docs/license.md`.

Case 1, a sibling. Move `docs/license.md` to `docs/terms.md`
while `notes/LICENSE` is listed. The count for `license` is 2,
the stem rewrite is skipped, and every `[[license]]` link still
points at the old name. The `newStem` check has the same false
positive. Such a sibling only enters `Files()` through a custom
`files:` glob.

Case 2, the moved file. `mdsmith move LICENSE COPYING` needs no
custom glob: `Resolve` reads any file on disk. The old stem is
`license` and only `docs/license.md` counts, so the guard
passes. Every `[[license]]` link, which resolves to
`docs/license.md`, is rewritten to `[[COPYING]]` and breaks.
If `LICENSE` is listed, the count is 2 today and the guard
hides this. A fix that only filters the count would expose it.

Case 3, a non-Markdown destination. Move `docs/guide.md` to
`docs/guide.mdx` while `a/guide.mdx` exists. The rewrite turns
`[[guide]]` into `[[guide.mdx]]`. A typed extension resolves by
exact file name, and `a/guide.mdx` sorts first, so the link
would land on the wrong file. Today the unfiltered count sees
`a/guide.mdx` under `guide.mdx` and skips the rewrite. A
Markdown-only count would drop that guard.

Round 2 of the PR #884 code review found case 1.

## Out of scope

The holder count reads `ws.Files()`. In the CLI that is
the `files:` globs minus gitignored paths. The wikilink
resolver walks every file on disk except `.git` and
`node_modules`. So a gitignored `archive/guide.md` can take
`[[guide]]` without being counted. Plan
[2610022104](2610022104_move-stem-count-resolver-index.md)
counts against the resolver's own index.

## Tasks

1. Add a failing row to `TestCountFilesWithStem` (now
   `TestWikilinkKeyHolders_OldStem`) in
   [move_coverage_test.go](../internal/refactor/move_coverage_test.go):
   an extensionless `notes/LICENSE` plus `docs/license.md`
   must give a count of 1 for `license`. Move the
   `non-markdown keeps extension` and `mdx keeps its extension`
   rows to the exact-name helper's test (task 6). Neither
   `img/api.png` nor `notes/b.mdx` is a stem target, but case 3
   still needs them counted by name.
2. Add a failing `Move` test for case 1. Moving
   `docs/license.md` while `notes/LICENSE` is in the workspace
   must still rewrite `[[license]]` to `[[terms]]`.
3. Add a failing `Move` test for case 2. Moving `LICENSE` to
   `COPYING` while `docs/license.md` and a `[[license]]` link
   exist must leave that link alone, both with `LICENSE`
   listed and unlisted.
4. Change `countFilesWithStem` so it skips any file that
   `mdpath.IsMarkdownPath` rejects. The wikilink index builds
   its stems with the same predicate.
5. Make `appendWikilinkStemEdits` return early when
   `mdpath.IsMarkdownPath(src)` is false: no `[[stem]]` link
   resolves to a non-Markdown file. Return early too when dst
   has no extension: a bare `[[name]]` finds Markdown files
   only, so no rewrite can reach it.
6. Add a failing `Move` test for case 3, then fix it. When dst
   has a non-Markdown extension, the destination guard counts
   workspace files whose lowercased basename equals dst's,
   not Markdown stems.
7. Fix the code-review findings on this change, each with a
   failing test first:

  - Keep the Markdown extension in the rewritten link when
     the destination stem holds a dot: `v1.3.md` is
     `[[v1.3.md]]`, since `[[v1.3]]` reads `.3` as a typed
     extension.
  - Skip the rewrite when the new spelling is empty
     (`docs/.md`) or contains `#`, `|`, `[`, or `]`. Such a
     link no longer parses as a link to dst.
  - Apply the `oldStem == newStem` early return only to a
     Markdown destination. Moving `guide.png.md` to
     `guide.png` must rewrite `[[guide.png.md]]`.
  - Count the moved file as a holder of its old stem even
     when `ws.Files()` omits it. One listed same-stem sibling
     then blocks the rewrite.
  - Replace `countFilesWithStem` and `countFilesWithName`
     with one pass, `wikilinkKeyHolders`.
  - Also skip the rewrite when the new spelling has a newline
     or a leading or trailing space. The target is trimmed, so
     `[[guide.md ]]` would reach another `guide.md`.
  - Skip a source with an empty stem (`docs/.md`). The
     whole-name fallback keyed it as `.md` and rewrote
     `[[.md.md]]` links to another file.
  - Normalize listed paths before the source-listed check, so
     a listed `./src` is not counted twice.
  - Key both ends and every holder with
     `linkgraph.FileStemKey`, the function `NewWikilinkIndex`
     keys files with, and retire `fileStem`. A listed
     `x/ guide.md` keys as ` guide` and no longer blocks a
     `[[guide]]` rewrite.
  - Replace the hand-written `#|[]` list with
     `linkgraph.WikilinkReaches`. It runs the spelling back
     through the wikilink grammar and the resolver's target
     checks, so a drive-letter name such as `C:x.md` is
     skipped too.
  - Pick the spelling with `WikilinkReaches`: the bare stem
     first, else the whole basename. This covers a dotted stem
     and a stem ending in a space (`[[guide .md]]`). Refuse a
     CR in the spelling, since CommonMark ends a line there.
  - Document these cases in
     [move.md](../docs/reference/cli/move.md).

## Acceptance Criteria

- [x] The stem holder count is 1, not 2, for `license`
      when the workspace holds `notes/LICENSE` and
      `docs/license.md`
- [x] Moving `docs/license.md` to `docs/terms.md` rewrites
      `[[license]]` even when an extensionless `LICENSE` is
      in the workspace
- [x] Moving `LICENSE` to `COPYING` leaves every `[[license]]`
      link to `docs/license.md` unchanged
- [x] Moving `docs/guide.md` to `docs/guide.mdx` leaves
      `[[guide]]` unchanged while `a/guide.mdx` exists
- [x] Moving `docs/v1.2.md` to `docs/v1.3.md` rewrites
      `[[v1.2.md]]` to `[[v1.3.md]]`
- [x] No wikilink is rewritten to an empty name, to one
      holding `#`, `|`, `[`, `]`, a CR, or a newline, or to one
      with a leading or trailing space
- [x] Moving `docs/api.md` to `docs/guide .md` rewrites
      `[[api]]` to `[[guide .md]]`
- [x] Moving `docs/.md` leaves `[[.md.md]]` unchanged
- [x] A listed `x/ guide.md` does not block moving
      `docs/guide.md` to `docs/manual.md`
- [x] Moving `docs/api.md` to `docs/C:x.md` leaves `[[api]]`
      unchanged
- [x] Moving an unlisted `a/b/guide.md` while
      `docs/guide.md` is listed leaves `[[guide]]` unchanged
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues
