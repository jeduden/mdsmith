---
id: 2610040606
title: Rewrite and count typed wikilinks on move
status: "✅"
summary: >-
  Make `mdsmith move` and `workspace/willRenameFiles` rewrite a
  typed `[[name.ext]]` wikilink to a moved non-Markdown file, and
  count one to a shadowed file in the batch warning, by giving the
  index an exact-name wikilink edge lookup.
model: opus
depends-on: [2610030438]
---
# Rewrite and count typed wikilinks on move

## Goal

A typed `[[img.png]]` link should follow `img.png` when it
moves, and a batch warning should count it when another file
takes `img.png`'s path.

## Background

The index keys wikilink edges by stem only.
`IncomingWikilinkEdges` in
[`index.go`](../internal/index/index.go) finds `[[guide]]`
and `[[guide.md]]`, but not `[[img.png]]`. The resolver
reads that link by exact file name.
[`appendWikilinkStemEdits`](../internal/refactor/move.go)
returns early for a source with no stem key. So a lone move
of `img.png` to `pics/photo.png` leaves `[[img.png]]` as
written, and the link breaks. A move that keeps the name,
such as to `pics/img.png`, needs no rewrite: the resolver
reads the base name alone.

Code review of PR #907 found the batch form of this gap.
`img.png` moves onto an existing `img2.png`, which is
refused. In the same batch, `a.png` moves to `img.png`.
[`countShadowed`](../internal/refactor/moveall.go) counts
only `[[stem]]` links. So `[[img.png]]` in `n.md` silently
reaches `a.png`, and the warning does not count it.
`TestMoveAll_ShadowedWithoutStem` locks this in with
`Withheld` 1.

An LSP explorer rename sends only Markdown files today, so
the batch case needs a client that sends other files.

## Tasks

1. Add an exact-name wikilink edge lookup to the index
   (for example `IncomingWikilinkNameEdges(name)`), keyed
   by the lowercased base name the resolver's typed lookup
   reads. Expose it on `refactor.MoveWorkspace`.
2. In `appendWikilinkStemEdits` (since renamed
   `appendWikilinkKeyEdits`), rewrite a typed
   `[[name.ext]]` link to a moved non-Markdown source when
   that source wins the name today and the new name reaches
   its destination once the batch has run.
3. In `countShadowed`, count a typed link to a shadowed
   non-Markdown file the way a `[[stem]]` link is counted.
   Update `TestMoveAll_ShadowedWithoutStem` to expect it.
4. Document the typed-link rewrite in
   [`move.md`](../docs/reference/cli/move.md).

## Acceptance Criteria

- [x] Moving `img.png` to `pics/photo.png` rewrites
      `[[img.png]]` only when it reached `img.png` before
      the move. A move that keeps the name, such as to
      `pics/img.png`, leaves it as written, as a kept
      `[[stem]]` is.
- [x] A batch that shadows `img.png` counts each typed
      `[[img.png]]` link in `Withheld`.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues

## Follow-ups

The round-1 code review found one design debt. The stem and
exact-name split is now spelled as paired APIs:

- `StemPaths` and `NamePaths` in
  [`wikilinks.go`](../internal/linkgraph/wikilinks.go).
- `EdgeWikilink` and `EdgeWikilinkName` in
  [`index.go`](../internal/index/index.go).
- The two `IncomingWikilink*Edges` lookups in `index.go`
  and [`indexedges.go`](../internal/refactor/indexedges.go).

Round 2 removed two parts of that debt. `WikilinkKeyAt`
replaced the paired `WikilinkStemAt` and `WikilinkNameAt`.
Wikilink edges now sit in `FileEntry.Wikilinks`, apart
from `Outgoing`, so views that list a file's edges need no
filter. Round 3 removed `WikilinkStem` and
`WikilinkName`: once the index build read `WikilinkKey`,
only their own tests called them.

The private `wikilinkKey` in
[`move.go`](../internal/refactor/move.go) picks one of each
pair by its `isStem` flag. One exported key type in
linkgraph would let a new wikilink feature be written once.
That changes three packages' APIs, so it is out of scope
here. File it as its own plan once `PLAN.md` has room. It
is still at the 300-line cap noted in plan 2610030438.
Joining its catalog lines to save room breaks the 80-column
limit.

The review also found a bug in the `<?include?>` link
rebase in
[`links.go`](../internal/rules/include/links.go). Its link
regex allows one level of nested brackets, so a link whose
label quotes a `[[stem]]` wikilink is never rebased. The
`move.md` row in `.github/copilot-instructions.md` hits it.
Fixing it changes what `<?include?>` generates, so the
pinned `mdsmith-fixed-version` job would flag the
regenerated file until a release carries the fix and the
pin is bumped (see [adopting new syntax][adopt]).
That puts it outside this PR. File it as its own plan once
`PLAN.md` has room, with the adoption step gated on the pin
bump.

[adopt]: ../docs/development/adopt-new-directive-syntax.md
