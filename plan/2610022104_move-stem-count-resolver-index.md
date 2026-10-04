---
id: 2610022104
title: Count wikilink stem siblings against the resolver's index on move
status: "✅"
summary: >-
  The move planner's `wikilinkKeyHolders` counts files from
  `ws.Files()`, but the wikilink resolver indexes every
  Markdown file on disk except `.git` and `node_modules`. A
  gitignored `archive/guide.md` can take `[[guide]]` yet is not
  counted, so moving `docs/guide.md` rewrites links that
  resolved to the archive. The Session walk counts
  `node_modules` READMEs the resolver skips, blocking safe
  rewrites. The stem count must use the same file set the
  resolver uses.
model: sonnet
depends-on: [2610022044]
---
# Count wikilink stem siblings against the resolver's index on move

## Goal

The move guard must count the files that `[[stem]]` links
read. Then a move never rewrites a link to a file the guard
did not see.

## Background

[move.go](../internal/refactor/move.go) decides whether to
rewrite `[[old]]` to `[[new]]` by counting workspace files
whose stem equals the old or new stem. It counts
`ws.Files()`. In the CLI that list is the `files:` globs minus
gitignored paths.

[wikilinks.go](../internal/linkgraph/wikilinks.go) resolves
`[[stem]]` through `NewWikilinkIndex`, which walks the whole
root except `.git` and `node_modules`.

Two failure cases follow:

- **Under-count (CLI):** a gitignored `archive/guide.md` is
  indexed by the resolver but not counted. Moving
  `docs/guide.md` to `docs/manual.md` counts 1 for `guide`
  and rewrites every `[[guide]]` to `[[manual]]`, including
  links that resolved to `archive/guide.md`.
- **Over-count (Session):** the Session walk includes
  `node_modules/*/README.md`, which the resolver skips, so a
  `[[readme]]` rewrite is blocked for no reason.

Round 4 of the PR #884 code review found this. It was left
out of plan 2610022044, which fixes the extension filter.

## Tasks

1. Add a failing `Move` test for the under-count case: a
   workspace whose file list omits `archive/guide.md` while
   the backing filesystem holds it. Moving `docs/guide.md`
   must leave `[[guide]]` unchanged.
2. Add a failing `Move` test for the over-count case:
   `node_modules/pkg/README.md` in the file list must not
   block the `[[readme]]` rewrite when `docs/readme.md` moves.
3. Expose the stem and name lookups on
   `linkgraph.WikilinkIndex` (`StemPaths`, `NamePaths`; the
   guard needs the paths, not only a count, to tell whether
   the source is among them) and have the move planner receive
   the index for the workspace root through
   `Workspace.WikilinkIndex`, replacing `wikilinkKeyHolders`'
   walk over `ws.Files()`.
4. Keep the exact-name guard from plan 2610022044 on the same
   index (`names` map) so both checks read one file set.
   The holder count then reads the index instead of
   `destResolver.paths`.
5. Block a non-Markdown destination when any file the
   resolver indexes has its name, not only a listed one. An
   unlisted root `logo.png` must block moving `docs/logo.md`
   to `docs/logo.png`, since `[[logo.png]]` would reach the
   shallower root file. The same holds in the LSP and session
   API, where `Files()` lists only `.md` files: an existing
   `a/guide.mdx` must block moving `docs/guide.md` to
   `docs/guide.mdx`. The PR #885 code review found this;
   [move.md](../docs/reference/cli/move.md) documents the
   current listed-only check.
6. `wikilinkStemBytes` re-implements `normalizeTarget`'s
   trim, slash, and base logic, so the rewrite range and the
   edge key can drift. Have `linkgraph` return the base
   segment's byte span and use it in the move planner. The
   PR #885 code review found tasks 6 to 9.
7. The edge loop strips the `./` that `dstWikilinkSpelling`
   adds and never checks the result. Return the bare
   spelling plus a needs-prefix flag, or check the prefixed
   token with `WikilinkReaches`.
8. `dstWikilinkSpelling` checks again whether dst is
   Markdown. Pass in `FileStemKey`'s answer instead.
9. `index.IncomingWikilinkEdges` keys its lookup with
   `strings.ToLower`. Use `linkgraph.FileNameKey`, which
   keys the stored edges.
10. The resolver never indexes a file under `.git` or
    `node_modules`, so a move there leaves `[[stem]]` as
    written instead of retargeting it at a name no wikilink
    reaches (`linkgraph.WikilinkIndexed`). A move out of one
    leaves it as written too: no `[[stem]]` reached the
    source, so none is retargeted at the moved file.
11. A workspace with no wikilink index (its root walk
    failed) counts its listed files through
    `linkgraph.NewWikilinkIndexFromPaths`, so a listed
    same-stem sibling still blocks the rewrite.
12. Before it edits the link at an incoming edge's column,
    the planner reads that link's stem key with
    `linkgraph.WikilinkStemAt` and skips it unless the key is
    still the old stem, so an edge from a stale index cannot
    rewrite a link to another file.
13. A `[[stem]]` reads the basename alone and reaches the
    shallowest, then alphabetically first, same-stem file.
    The guard used to skip the rewrite whenever another file
    shared the old stem. When the moved file is that first
    file, every link then silently reached the sibling. The
    guard now asks `WikilinkIndex.StemResolvesTo` whether the
    moved file is the one `[[stem]]` reaches, and rewrites
    every such link when it is. The PR #904 pre-merge review
    found this.
14. The PR #904 verification review tightened that guard
    three ways. A source path typed in other letter case
    than an indexed holder is not taken to win the stem, as
    a case-insensitive file system may hold it under that
    spelling. A link whose folder prefix names a sibling's
    folder (`[[ref/Guide]]`) is left as written. The new
    stem or name blocks the rewrite only when a holder sorts
    before the destination (`NameResolvesTo` for a typed
    name).

PR #885 already retired `fileStem`: the move planner keys
files with `linkgraph.FileStemKey`, the same function
`NewWikilinkIndex` uses.

## Acceptance Criteria

- [x] A gitignored Markdown file that `[[stem]]` resolves to
      blocks the stem rewrite on move
- [x] A `node_modules` README does not block a `[[readme]]`
      rewrite on move
- [x] An unlisted `logo.png` blocks moving `docs/logo.md` to
      `docs/logo.png`
- [x] The move planner's stem counts come from the same index
      `[[stem]]` resolution reads
- [x] A move into or out of `node_modules` or `.git` leaves
      `[[stem]]` as written
- [x] With no wikilink index, a listed same-stem sibling
      that `[[stem]]` reaches still blocks the rewrite
- [x] Moving the same-stem file that `[[stem]]` reaches
      rewrites every `[[stem]]`; moving one a sibling
      outsorts rewrites none
- [x] A `[[stem]]` whose folder prefix names a sibling's
      folder is left as written
- [x] A file holding the new stem blocks the rewrite only
      when it sorts before the destination
- [x] A stale edge whose column holds a link to another
      stem is not rewritten
- [x] The move planner reuses `linkgraph`'s base-segment
      span, Markdown test, and name key, with no copies
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues
