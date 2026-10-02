---
id: 2610022044
title: Count only Markdown files as wikilink stem siblings on move
status: "🔲"
summary: >-
  The move planner's wikilink pass treats a non-Markdown file
  (for example an extensionless `LICENSE`) as a stem target,
  though the wikilink index only maps `.md`/`.markdown` files
  to stems. A listed `LICENSE` blocks the `[[license]]`
  rewrite when `docs/license.md` moves, and moving `LICENSE`
  itself rewrites `[[license]]` links that point at
  `docs/license.md`. Only Markdown files may count or move as
  stem targets.
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

Round 2 of the PR #884 code review found case 1.

## Tasks

1. Add a failing row to `TestCountFilesWithStem` in
   [move_coverage_test.go](../internal/refactor/move_coverage_test.go):
   an extensionless `notes/LICENSE` plus `docs/license.md`
   must give a count of 1 for `license`. Flip the
   `non-markdown keeps extension` and `mdx keeps its extension`
   rows to 0: neither `img/api.png` nor `notes/b.mdx` is a
   stem target.
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
   resolves to a non-Markdown file.

## Acceptance Criteria

- [ ] `countFilesWithStem` returns 1, not 2, for `license`
      when the workspace holds `notes/LICENSE` and
      `docs/license.md`
- [ ] Moving `docs/license.md` to `docs/terms.md` rewrites
      `[[license]]` even when an extensionless `LICENSE` is
      in the workspace
- [ ] Moving `LICENSE` to `COPYING` leaves every `[[license]]`
      link to `docs/license.md` unchanged
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues
