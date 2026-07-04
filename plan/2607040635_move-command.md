---
id: 2607040635
title: >-
  Add mdsmith move: move a file and rewrite every
  incoming reference
status: "🔲"
model: opus
summary: >-
  New `mdsmith move <src> <dst>` subcommand. Moves the
  file via `git mv` when it is tracked in the current
  Git repository, plain rename otherwise, and rewrites every
  incoming workspace reference (links, ref-defs,
  wikilinks, include/build paths) in one atomic
  operation. Closes gap C-4/L-3 from the mdbase
  comparison.
---
# Add mdsmith move

## Context

[mdsmith rename](../docs/reference/cli/rename.md) covers
headings and link-reference labels but not file moves.
After a `git mv`, MDS027 flags every broken incoming
link, yet nothing rewrites them. The gap is catalogued
as C-4/L-3 in
[learn-from-mdbase.md](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md)
and surfaced again in the PR #718 discussion of the
[linter comparison](../docs/background/markdown-linters.md).
mdbase ships this today; it is the one rename surface
mdsmith lacks.

## Goal

`mdsmith move <src> <dst>` moves a workspace file and
rewrites every incoming reference in one step. A file
move never leaves a broken link behind.

## Design decisions

- **Move mechanics.** When `<src>` is tracked in the
  current Git repository, exec `git mv` so the index
  records the rename. Otherwise use a plain filesystem
  rename. A
  failing `git mv` aborts the whole operation; no
  half-moved state.
- **Rewrites.** Incoming `[text](path)` and
  `[text](path#anchor)` links, `[label]: path`
  ref-defs, `<?include?>` `file:` paths, and
  `<?build?>` input paths. Wikilinks `[[stem]]` are
  rewritten only when the basename stem changes.
  Front-matter path fields are out of scope, matching
  the C-4 open-questions call.
- **Safety.** Same contract as `mdsmith rename`: both
  paths workspace-relative with traversal rejected, an
  existing `<dst>` aborts, any conflict exits 2 with
  no partial edit written. `--dry-run` prints the
  proposed edits. Output is text or JSON; exit codes
  are 0 moved, 1 source not found, 2 error.
- **Posture.** The only new subprocess is the local
  `git` binary, invoked only when a work tree is
  detected. No network use; the
  [telemetry stance](../docs/reference/telemetry.md)
  is unchanged.

## Tasks

1. Red/green: unit tests for git-tracked detection
   (temp git repo, tracked / untracked / no-repo
   cases), then the detection helper.
2. Red/green: move engine in a package next to
   `internal/rename` — filesystem path first, then the
   `git mv` path, then abort-on-conflict behavior.
3. Red/green: incoming-reference rewrite reusing the
   dependency-graph index behind
   [mdsmith deps](../docs/reference/cli/deps.md);
   cover links, anchors, ref-defs, wikilinks, include
   and build paths.
4. Wire the `mdsmith move` CLI subcommand with
   `--dry-run`, `--format`, and the shared walk flags;
   add integration tests.
5. Write `docs/reference/cli/move.md` and extend
   [the rename feature page](../docs/features/rename.md)
   with the move surface.
6. Update the mdbase section of
   [the comparison page](../docs/background/markdown-linters.md)
   and mark C-4/L-3 as scheduled by this plan in the
   gap catalogue.
7. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [ ] `mdsmith move a.md b/c.md` on a tracked file
      stages the rename in git and rewrites every
      incoming reference.
- [ ] The same command outside git (or on an
      untracked file) moves the file and rewrites the
      same references.
- [ ] An existing destination, a traversal path, or a
      failed `git mv` exits 2 with no file moved and
      no edit written.
- [ ] `--dry-run` prints the edits and changes
      nothing.
- [ ] `mdsmith check` reports no MDS027 diagnostics
      after a move performed by the command.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint
      run` reports no issues.
- [ ] `mdsmith check .` — 0 failures.
