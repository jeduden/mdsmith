---
id: 2607040822
title: >-
  Refactor engine redesign: unify move and rename
  behind one Plan, across CLI, LSP, and WASM hosts
status: "🔲"
model: opus
summary: >-
  Ground-up redesign of mdsmith's reference-rewriting
  commands. `move` and `rename` become two verbs over one
  engine. Each rewrites every reference when a symbol's
  identity changes: a file path, a heading slug, or a
  link-ref label. One `internal/refactor` engine emits a
  neutral Plan — per-file edits plus an optional file
  operation. The CLI, the LSP server, and the WASM engine
  each execute that Plan their own way. Supersedes the
  narrow move-command plan and closes gap C-4/L-3.
depends-on: []
---
# Refactor engine redesign

## Context

[mdsmith rename](../docs/reference/cli/rename.md) rewrites
headings and link-reference labels across the workspace.
[mdsmith deps](../docs/reference/cli/deps.md) exposes the
same edge graph. A file *move* is the one rename surface
mdsmith lacks. After a `git mv`, MDS027 flags every broken
incoming link, and nothing rewrites them. The gap is C-4/L-3
in
[learn-from-mdbase.md](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md).

The superseded [move-command plan](2607040635_move-command.md)
bolted a standalone `move` onto the side of `rename`. That
duplicates the whole apply, atomic-write, dry-run, and
exit-code layer, plus the path-rewrite logic already near the
anchor rewrite in
[internal/rename](../internal/refactor/heading.go). This plan
treats move and rename as one operation. There is no
backwards-compatibility constraint on the CLI or LSP.

## Goal

One engine serves every identity change a workspace can
undergo. A file move, a heading rename, and a label rename
share one safety contract, one edit model, and one set of
host adapters. A move never strands a link in either
direction, and never leaves a broken directive path behind.

## The core model

A workspace is a graph of addressable symbols. Exactly three
identities can change. Each change must rewrite every
reference that resolved to the old identity:

| Symbol         | Identity           | Referenced by                                                                | CLI command                   | LSP request                 |
| -------------- | ------------------ | ---------------------------------------------------------------------------- | ----------------------------- | --------------------------- |
| File           | workspace-rel path | file-links, ref-def dests, `include`/`build`/`catalog` paths, wikilink stems | `mdsmith move`                | `workspace/willRenameFiles` |
| Heading        | `(file, slug)`     | anchor-links, ref-def anchors, same-file `(#slug)`                           | `mdsmith rename --as heading` | `textDocument/rename`       |
| Link-ref label | `(file, label)`    | `[text][label]`, shortcut `[label]`                                          | `mdsmith rename --as label`   | `textDocument/rename`       |

Move and rename are one operation at the engine layer. They
differ only in which identity kind changes. Two exist today,
as `rename`; move is the missing third. The surface stays two
commands, so a reader never concludes one command does both.

The split between *planning* and *executing* is the
load-bearing idea. The engine is pure: it returns edits and a
described file operation, and never touches the filesystem.
Each host then executes the operation — the CLI runs
`git mv`, the LSP lets the editor rename, Obsidian calls the
vault API. So `git mv` cannot live in the engine: it is a
subprocess, and there are none under `GOOS=js GOARCH=wasm`
([engine API](../docs/background/concepts/engine-api.md)).

## Package structure

Rename [internal/rename](../internal/refactor/rename.go) to
`internal/refactor`. It already speaks a surface-neutral
`Edit` / `Position` / `Range` vocabulary and owns a
[`Workspace` seam](../internal/refactor/heading.go), so this
widens scope rather than rewriting. Add:

- `refactor.Plan` — `Edits` keyed by output target (CLI path
  or LSP URI), a described `FileOp` (nil for a rename), and a
  typed `Conflict error`.
- `refactor.Move(ws, src, dst)` — the path identity change.
- `Heading` and `LinkRef` — same behavior, returning a
  `Plan` so all three share one shape.

The CLI apply layer in
[rename.go](../cmd/mdsmith/rename.go) becomes a shared
`applyPlan` both commands call. The LSP adapter in
[lsp/rename.go](../internal/lsp/rename.go) is reused by
`rename` and the new file-operation handler.

The [index](../internal/index/index.go) already stores the
edges a move needs, each with a resolved target file. Move
adds `IncomingPathEdges(file)` — every edge whose target is
`file`, ignoring the anchor. It also adds an `EdgeWikilink`
kind keyed by basename stem.

## Move engine behavior

`refactor.Move(ws, src, dst)` returns a Plan with five parts.

1. **Incoming path edges.** For each edge in file *F*
   pointing at `src`, rewrite only the path token to the path
   relative from `dir(F)` to `dst`. A `[t](src#sec)` link
   keeps its fragment. `include`, `build` `inputs:`, and
   resolvable `catalog` entries move the same way.
2. **Ref-def destinations.** A companion pass over
   `[label]: src` definitions, mirroring the heading ref-def
   pass in [heading.go](../internal/refactor/heading.go).
3. **Wikilink stems.** `[[old-stem]]` becomes `[[new-stem]]`
   only when the basename stem differs. A move that keeps the
   basename thus updates path links but leaves wikilinks
   alone (a stem still resolves) — an asymmetry `--dry-run`
   and the docs must state.
4. **Outbound relative links in the moved file.** Recompute
   every relative link inside `src` so it resolves from
   `dir(dst)`. Skip this and moving `docs/a.md` to
   `guide/a.md` breaks its own `[x](./b.md)`. A correct move
   fixes both directions.
5. **`FileOp{From, To}`** — described, not run.

Recomputation preserves spelling: `./x.md` stays relative,
`/docs/x.md` stays root-anchored, and absolute URLs, mailto,
and non-workspace paths are never touched.

### Safety contract (shared by move and rename)

- Both paths are workspace-relative; traversal is rejected.
- An existing `dst` aborts, and nothing moves.
- Any conflict — a slug or label collision, a failed
  `git mv` — aborts the whole operation with no partial edit.
- `--dry-run` prints the edits and the planned operation and
  changes nothing.
- Output is `text` or `json`; exit 0 done, 1 not found, 2
  error or conflict.
- Writes are atomic and preserve mode, reusing
  [writeFilePreservingMode](../cmd/mdsmith/rename.go).
- On the CLI, text edits apply before the `git mv`; the LSP
  inverts this — the editor applies the edit, then renames.

## CLI

```text
mdsmith move   [flags] <src> <dst>
mdsmith rename [flags] <file> <old> <new>
```

`rename` drops the `--heading` / `--link-ref` booleans for a
single `--as heading|label`. When `--as` is omitted, the
engine auto-detects: a heading whose visible text equals
`<old>`, or a label normalizing to `<old>`. If exactly one
matches it is used; if both or neither, the command exits 2
and asks for `--as`. Both commands share the config, format,
dry-run, and walk flags the current rename and
[deps](../cmd/mdsmith/deps.go) already expose.

## The naming collision: a file rename is `move`

A panel of six simulated developers read this design cold and
answered an eight-question scenario quiz. Every one scored
8/8 on the semantics, including the subtle outbound-link and
WASM points. So the engine model lands. But all six flagged
the same trap.

**Changing a file's basename is `move`, not `rename`** — it
is a path change. Yet every human signal says otherwise: F2
is labeled "Rename", Obsidian says "rename note", English
says "rename the file". A user who wants to rename `api.md`
to `service.md` will type `mdsmith rename` and be wrong;
`rename` only touches a heading or label *inside* a file. The
LSP inherits the overload — a file rename is
`willRenameFiles`, a heading rename is `textDocument/rename`.

The redesign keeps the two-verb split but stops leaning into
the collision. `rename` guards against move intent. When
`<old>` or `<new>` looks like a path, or `<file>` holds no
matching symbol, it exits 2 and names `mdsmith move`. The
docs then lead with a decision table:

| You want to…                                   | Command  |
| ---------------------------------------------- | -------- |
| Relocate or rename a *file* (path or basename) | `move`   |
| Retitle a *heading* and fix its anchors        | `rename` |
| Rename a *link-ref label* and its uses         | `rename` |

## LSP requirements

The LSP already mirrors symbol renames through
[textDocument/rename](../docs/reference/cli/lsp.md). Move is
its own request, so the split is native to the protocol.

- Advertise `workspace.fileOperations.willRename` in
  `initialize`, filtered to `**/*.{md,markdown}`.
- Handle `workspace/willRenameFiles`: run `refactor.Move`
  against the warm index and overlay buffers, return the
  merged `WorkspaceEdit` (reusing the `textDocument/rename`
  adapter). The client applies it, then renames.
  `didRenameFiles` swaps the path in the index.
- Leave `textDocument/rename` and `prepareRename` unchanged.
  A file rename is never a `textDocument/rename`, so
  `prepareRename` returns null there.
- The registration is inert unless the client advertises the
  matching capability; document that contract.

## VS Code extension requirements

The [LanguageClient wiring](../editors/vscode/src/wiring.ts)
mostly exists: `vscode-languageclient@9` forwards
`willRenameFiles` once the server registers it.

- Confirm the client advertises the capability (it does by
  default); add a wiring test so a future trim cannot regress
  it. Filters are glob-based, so renames fire for any `.md`.
- No new command is required: an explorer rename or drag
  fires `willRenameFiles`. Add an e2e test asserting the
  links were rewritten. F2 already renames headings and
  labels; an optional "Move File…" palette entry can follow.

## Obsidian extension requirements

The Obsidian plugin has no LSP; it drives the WASM
[engine `Session`](../docs/background/concepts/engine-api.md)
over the vault's `MemWorkspace`.

- Mirror `rename` and a plan-only `move` on the Go `Session`
  and the JS proxy, feature-detected via `capabilities()`.
  `move` returns a Plan; the host performs the vault rename,
  since `git mv` is build-tagged out of WASM.
- Hook `vault.on('rename', …)`. Obsidian already fixes
  standard links and wikilinks, but not mdsmith directive
  paths or slug `#anchor` fragments. The plugin rewrites that
  subset. A setting chooses whether mdsmith owns all rewrites
  or only the ones Obsidian misses, so the two never
  double-edit a link.
- Add a "Rename heading" command that runs `Session.rename`
  and applies the plan, matching the whole-buffer
  [fix flow](../editors/obsidian/src/actions.ts).
- `internal/refactor` must keep compiling under both wasm
  targets, within the ≤ 18 MB and ≤ 8 MB budgets.

## Tasks

1. Rename `internal/rename` to `internal/refactor`;
   introduce `Plan` and `FileOp`; make `Heading` / `LinkRef`
   return a `Plan`. Red/green; no behavior change.
2. Extend the index: `IncomingPathEdges` and an
   `EdgeWikilink` kind with its collector.
3. `refactor.Move` planner: incoming path edges, ref-def
   dests, wikilink stems, outbound relative links, anchor
   preservation, abort-on-conflict.
4. Native `FileOp` behind `//go:build !wasm`: git detection,
   `git mv`, plain-rename fallback. Temp-repo tests.
5. Shared `applyPlan`; add `mdsmith move`; redesign `rename`
   to `--as`; add the move-intent guard. Integration tests.
6. LSP: `willRenameFiles` / `didRenameFiles`, the
   `fileOperations` capability, index refresh. Three-file
   test.
7. Engine API: mirror `rename` and plan-only `move`; extend
   `capabilities()`; WASM smoke and size tests.
8. VS Code: capability wiring test and rename e2e. Obsidian:
   the rename hook, "Rename heading" command, ownership
   setting, tests.
9. Docs: rewrite
    [rename.md](../docs/reference/cli/rename.md), add
    `move.md`, broaden
    [the feature page](../docs/features/rename.md) with the
    decision table, and update
    [lsp.md](../docs/reference/cli/lsp.md),
    [vscode-extension.md](../docs/reference/vscode-extension.md),
    the [Obsidian guide](../docs/guides/editors/obsidian.md),
    and [engine-api.md](../docs/background/concepts/engine-api.md).
    Update the mdbase
    [comparison](../docs/background/markdown-linters.md) and
    mark C-4/L-3 scheduled.
10. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [ ] `mdsmith move a.md b/c.md` on a tracked file stages the
      git rename and rewrites every incoming reference, plus
      the outbound relative links inside the moved file.
- [ ] The same command outside git moves the file and
      rewrites the same references.
- [ ] An existing destination, a traversal path, or a failed
      `git mv` exits 2 with nothing moved or written.
- [ ] `mdsmith rename` renames a heading or label with `--as`
      or auto-detect; conflicts exit 2 and name the collision.
- [ ] A path-shaped `rename` (or one on a missing symbol)
      exits 2 and names `mdsmith move`.
- [ ] `--dry-run` changes nothing for both commands, and
      `mdsmith check` reports no MDS027 after a command move.
- [ ] The LSP advertises `workspace.fileOperations.willRename`
      and answers `willRenameFiles` with a covering edit.
- [ ] `Session` exposes `rename` and plan-only `move`,
      mirrored in JS and reported by `capabilities()`.
- [ ] VS Code rewrites links on an explorer rename; Obsidian
      rewrites the references it misses on a vault rename.
- [ ] `go test ./...`, `golangci-lint`, both WASM budgets,
      and `mdsmith check .` all pass.

## Open Questions

- **Obsidian rewrite ownership.** Default to mdsmith owning
  only the directive/anchor subset, or everything? The
  setting covers both; the default is open.
- **Front-matter path fields.** Still out of scope, matching
  C-4. Revisit if a schema type marks a field as a path.
- **`--as` ambiguity.** When a heading and label share text,
  the command errors and asks for `--as`. Confirm erroring
  beats a precedence order.
- **Guard aggressiveness.** The `.md`-suffix heuristic trips
  a heading titled `config.md`. Confirm it is worth the rare
  false positive, or gate on the missing-symbol case alone.
