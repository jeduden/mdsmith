---
id: 151
title: LSP rename for headings and link-reference labels
status: "✅"
model: opus
summary: >-
  Add `textDocument/prepareRename` and
  `textDocument/rename` to `mdsmith lsp` so an editor
  or agent can rename a heading and have every
  workspace anchor link rewritten in one
  `WorkspaceEdit`. Also covers link-reference label
  rename within the current file.
---
# LSP rename for headings and link-reference labels

## Goal

Let an LSP client rename a heading from one
buffer. Every workspace anchor link that points
at it (`[text](file.md#anchor)`,
`[text](#anchor)`) updates in one atomic
`WorkspaceEdit`. The same flow handles link-ref
label renames. Each shortcut and full reference
use in the file updates with the def.

Without this, an agent that restructures Markdown
breaks anchors silently. MDS027
(cross-file-reference-integrity) catches the
breakage on the next lint pass, but only after the
fact.

## Background

Plan 131 shipped the workspace symbol index plus
`definition`, `references`, `implementation`,
and call hierarchy. Rename was a non-goal there.
Heading rewrites need anchor fixups across the
workspace. The team wanted that work in its own
plan.

The index already knows every edge it needs:

- Headings keyed by `(file, anchor)`.
- Anchor links keyed by `(file, anchor)`.
- Link-ref defs and uses keyed by `(file, label)`.

Slug computation uses
[`mdtext.CollectTOCItems`](../internal/mdtext/mdtext.go).
The same function disambiguates duplicate slugs by
appending `-1`, `-2`, …; rename has to recompute
the full slug map for the file to know whether the
new heading text collides with an existing one.

## Non-Goals

- File rename (`workspace/willRename` /
  `didRename`). Separate concern; rewrites the
  link path, not the anchor.
- Renaming a `kind:` value or a `kinds:` list
  entry. Kinds live in
  [.mdsmith.yml](../.mdsmith.yml); editing the
  config from an LSP rename would conflict with
  the user-consent rule in
  [CLAUDE.md](../CLAUDE.md).
- Renaming a directive name. Few directives, low
  value.
- Renaming front-matter keys. Schema work owns
  that.
- Markdown reflow after rename. The edit replaces
  the old heading text with the new text only;
  surrounding paragraph wrapping is left to the
  user or to `mdsmith fix`.

## Design

### Capability

```jsonc
"renameProvider": {
  "prepareProvider": true
}
```

`prepareProvider` is true because the rename range
for a heading excludes the leading `#`s and the
trailing closing `#`s (if any). The client needs
the explicit range so the rename popup highlights
only the heading text.

### `textDocument/prepareRename`

Returns the rename range for the symbol under the
cursor:

| Cursor on…                    | Range returned                                    |
|-------------------------------|---------------------------------------------------|
| Heading text                  | The text run between leading and trailing markers |
| `[label]: url` definition     | The label text inside `[…]`                       |
| `[text][label]` reference use | The label text inside `[…][label]`                |
| Anywhere else                 | `null` (rename not supported here)                |

Setext headings (`=====` / `-----`) are also in
scope: the range is the heading line, the
underline stays as-is.

### `textDocument/rename`

Computes a `WorkspaceEdit` per symbol kind.

#### Heading rename

1. Recompute the file's full slug map under the
   new heading text using
   [`mdtext.CollectTOCItems`](../internal/mdtext/mdtext.go).
2. Map old slug → new slug. If the new slug
   collides with another heading in the file
   (because the disambiguator now resolves
   differently), return an LSP error
   (`InvalidParams` with a `data` field naming the
   colliding heading) rather than silently
   shifting numbered suffixes. Slug collisions
   are rare and surprising; failing loud is
   better than corrupting cross-file links.
3. Build the `WorkspaceEdit`:

  - One `TextEdit` on the heading line in the
     current file.
  - One `TextEdit` per workspace anchor link
     pointing at `(file, oldSlug)`. The match
     uses the same edge table that powers
     `textDocument/references` for headings.
  - Same-file anchor links (`[text](#oldSlug)`)
     are included.
  - Setext heading renames touch only the text
     line; the underline keeps its byte length
     even if the text length changes (CommonMark
     does not require the underline width to
     match the text).

#### Link-ref label rename

1. The rename is file-local — link-ref defs and
   their uses do not cross files in CommonMark.
2. Build the `WorkspaceEdit`:

  - One edit on the `[label]: url` definition.
  - One edit per `[text][label]` and shortcut
     `[label]` use in the same file.

3. Collisions: if the new label matches another
   defined label, return `InvalidParams`. MDS029
   (no-unused-link-definitions) and MDS028
   (no-undefined-reference-labels) would surface
   the breakage anyway, but the LSP error catches
   it before the edit applies.

### Disambiguator handling

`mdtext.CollectTOCItems` appends `-1`, `-2`, …
when two headings share a base slug. A rename can
shift those numbers:

```markdown
## Setup        <!-- slug "setup" -->
## Setup        <!-- slug "setup-1" -->
```

Renaming the first to "Configuration" shifts the
second's slug from `setup-1` to `setup`. Any
anchor pointing at `#setup-1` silently breaks.
The plan addresses this two ways:

- The slug-collision check in step 2 of heading
  rename catches the case where the rename makes
  the *new* heading's slug collide with another.
- A separate "shift detection" pass walks the
  slug map after the rename and finds any heading
  whose slug changed even though it wasn't
  renamed. Each shifted slug becomes another
  `TextEdit` rewriting links pointing at the old
  slug to the new one. This keeps the workspace
  consistent.

The integration test covers a three-heading file
with a duplicate-name pair to verify shift
detection.

### Position and performance

Rename reuses the index from plan 131. Cost:

- `prepareRename`: O(1) — single position lookup.
- `rename` (heading): O(headings in file) for
  slug recompute, plus O(workspace anchor links
  to this file) for the edge walk. The 1 000-file
  benchmark in
  [`internal/index/bench_test.go`](../internal/index/bench_test.go)
  upper-bounds this at well under 100 ms.
- `rename` (link-ref): O(uses in current file)
  only.

### Backwards compatibility

`renameProvider` is additive. Clients that ignore
the capability see the post-plan-134 server.

## Tasks

1. Add `prepareRename` to the server
   ([`internal/lsp/server.go`](../internal/lsp/server.go))
   dispatching on the position-tag from
   [`internal/index/locate.go`](../internal/index/locate.go).
   Return null for unsupported positions.
2. Implement heading rename. Use
   `mdtext.CollectTOCItems` for slug recomputation.
   Surface slug-collision errors via
   `InvalidParams`. Cover same-file and cross-file
   anchor edges.
3. Implement shift detection for the
   disambiguator case. Add a unit test with two
   headings sharing a base slug.
4. Implement link-ref label rename. Cover full
   `[text][label]` and shortcut `[label]` uses.
5. Advertise `renameProvider.prepareProvider =
   true` in the `initialize` capabilities.
6. Add an end-to-end integration test in
   [`cmd/mdsmith`](../cmd/mdsmith) driving
   `initialize` → `didOpen` (across three files)
   → `prepareRename` → `rename` → assert the
   resulting `WorkspaceEdit` covers every expected
   edge.
7. Add a "Rename" section to
   [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
   with the prepareRename range table and the
   collision-error contract.

## Acceptance Criteria

- [x] `renameProvider.prepareProvider = true`
      appears in the `initialize` capabilities.
- [x] `prepareRename` on a heading returns the
      heading text range (excluding leading `#`s).
- [x] `prepareRename` on plain prose returns
      `null`.
- [x] `rename` on a heading rewrites the heading
      text in the current file and every anchor
      link in the workspace pointing at the old
      slug.
- [x] `rename` triggers shift detection: when a
      duplicate-name disambiguator changes,
      affected anchors update too.
- [x] `rename` on a link-ref definition rewrites
      every `[text][label]` and shortcut `[label]`
      use in the same file.
- [x] A heading rename whose new slug collides
      with an existing heading in the same file
      fails with an LSP `InvalidParams` error
      naming the colliding heading.
- [x] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      documents `renameProvider` and the
      collision contract.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.

## Open Questions

- **Cross-file link-ref defs.** Some Markdown
  flavors (not CommonMark) allow shared link-ref
  files. The plan keeps link-ref rename
  file-local; revisit if a future flavor profile
  introduces cross-file refs.
- **Annotation behavior.** A `WorkspaceEdit` can
  include `changeAnnotations` so the client shows
  a confirmation dialog ("rename heading and 47
  anchor links?"). The first pass returns a flat
  edit; add annotations later if reviewer
  feedback requests them.

## ...

<?allow-empty-section?>
