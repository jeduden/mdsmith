---
id: 2610040606
title: Rewrite and count typed wikilinks on move
status: "🔲"
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
of `img.png` to `pics/img.png` leaves `[[img.png]]` as
written. That link still resolves only while no other
`img.png` exists.

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
   reads. Expose it on `refactor.Workspace`.
2. In `appendWikilinkStemEdits`, rewrite a typed
   `[[name.ext]]` link to a moved non-Markdown source when
   that source wins the name today and the new name reaches
   its destination once the batch has run.
3. In `countShadowed`, count a typed link to a shadowed
   non-Markdown file the way a `[[stem]]` link is counted.
   Update `TestMoveAll_ShadowedWithoutStem` to expect it.
4. Document the typed-link rewrite in
   [`move.md`](../docs/reference/cli/move.md).

## Acceptance Criteria

- [ ] Moving `img.png` to `pics/img.png` rewrites
      `[[img.png]]` only when it reached `img.png` before
      the move.
- [ ] A batch that shadows `img.png` counts each typed
      `[[img.png]]` link in `Withheld`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
