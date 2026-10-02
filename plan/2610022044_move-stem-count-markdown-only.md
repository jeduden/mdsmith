---
id: 2610022044
title: Count only Markdown files as wikilink stem siblings on move
status: "🔲"
summary: >-
  `countFilesWithStem` in the move planner counts an
  extensionless workspace file (for example `LICENSE`) as a
  same-stem sibling, though the wikilink index only maps
  `.md`/`.markdown` files to stems. A move then skips the
  `[[stem]]` rewrite and leaves those links broken. Restrict
  the count to files a wikilink can resolve to.
model: sonnet
depends-on: []
---
# Count only Markdown files as wikilink stem siblings on move

## Goal

On a move, count only `.md` and `.markdown` files (any case)
as same-stem siblings. Those are the only files the wikilink
index resolves.

## Background

[move.go](../internal/refactor/move.go) has `fileStem`. If
`linkgraph.WikilinkStem` rejects a basename, `fileStem` falls
back to the whole basename in lower case. So a file named
`notes/LICENSE` counts as a sibling of `docs/license.md`.

Move `docs/license.md` to `docs/terms.md`. The count for
`license` is 2, the stem rewrite is skipped, and every
`[[license]]` link still points at the old name. The `newStem`
check has the same false positive.

Such a file only enters the workspace through a custom
`files:` glob, so the case is rare. Round 2 of the PR #884
code review found it.

## Tasks

1. Add a failing row to `TestCountFilesWithStem` in
   [move_coverage_test.go](../internal/refactor/move_coverage_test.go):
   an extensionless `notes/LICENSE` plus `docs/license.md`
   must give a count of 1 for `license`.
2. Add a failing `PlanMove` test. Moving `docs/license.md`
   while `notes/LICENSE` is in the workspace must still
   rewrite `[[license]]` to `[[terms]]`.
3. Change `countFilesWithStem` so it skips any file that
   `mdpath.HasMarkdownExt` rejects. Keep `fileStem` as it is,
   because its other callers need the whole-basename
   fallback.

## Acceptance Criteria

- [ ] `countFilesWithStem` returns 1, not 2, for `license`
      when the workspace holds `notes/LICENSE` and
      `docs/license.md`
- [ ] Moving `docs/license.md` to `docs/terms.md` rewrites
      `[[license]]` even when an extensionless `LICENSE` is
      in the workspace
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues
