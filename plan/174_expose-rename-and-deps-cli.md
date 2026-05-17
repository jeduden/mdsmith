---
id: 174
title: Expose rename and dependency-graph as CLI subcommands and feature docs
status: "🔳"
model: opus
depends-on: [131, 151, 153]
summary: >-
  Lift the heading / link-reference rename logic out of
  `internal/lsp` into a shared `internal/rename` core, relocate
  the LSP symbol index to `internal/index`, and expose rename
  plus the include/catalog/build/link dependency graph as the
  name-based `mdsmith rename` and `mdsmith deps` subcommands.
  Document every LSP capability as a feature.
---
# Expose rename and dependency-graph as CLI subcommands and feature docs

## Goal

The rename capability (plan 151) ships only over the LSP
wire protocol. So does the call-hierarchy / dependency graph
(plan 131). An agent or script with no editor cannot reach
either. Neither appears in `docs/features/`.

This plan exposes both as CLI subcommands. It also documents
the full LSP surface as features. Domain logic is not
duplicated.

## Background

Today `internal/lsp/rename.go` computes slug remaps and
edits as methods on the LSP `*Server`. It speaks LSP wire
types (`WorkspaceEdit`, `textEdit`). The
[layering map](../docs/development/architecture/index.md)
makes `cmd/mdsmith` and `internal/lsp` sibling entry points.
So the CLI must not import `internal/lsp`.

Plan 153 made `internal/linkgraph` the canonical shared link
extractor. `mdsmith list backlinks` already consumes it with
no `internal/lsp` import. Plan 153 kept the symbol index
LSP-local on purpose.

This plan supersedes that one plan-153 non-goal. The symbol
index moves to a peer `internal/index` package. Both entry
points can then consult it. The
[architecture audit log](../docs/development/architecture-audit.md)
records the supersession.

## Non-Goals

- File rename, `kind:` rename, directive-name rename,
  front-matter-key rename — all out of scope per plan 151.
- New CLI commands for the inherently interactive navigation
  capabilities (definition, references, document-symbol,
  workspace-symbol, implementation, completion). They get
  feature-doc coverage only; `cross-system.md` warns against
  manufacturing CLI surfaces for editor-only features.
- Changing the LSP wire behavior. Plans 131 / 151 test suites
  are the regression gate for the delegation refactor.
- A persistent graph cache. `deps` builds a transient index
  per invocation, like `backlinks`.

## Design

### Package boundaries

- `internal/index` — the relocated symbol / edge index
  (pure `git mv` of `internal/lsp/index`; package name
  unchanged, import path only). Support layer; consumed by
  `internal/lsp`, `internal/rename`, and `cmd/mdsmith`.
  Must not import `internal/lsp` (SRP, DIP).
- `internal/rename` — NEW core. Answers one question:
  "given a workspace and a rename target, what file edits
  perform it, or what typed error?" Depends on
  `internal/linkgraph`, `internal/mdtext`, `internal/index`.
  Returns plain `Edit{File,Start,End,NewText}` and typed
  errors; no LSP types. Must not import `internal/lsp`
  (DIP — high-level surfaces depend on this, not the
  reverse).
- `internal/lsp/rename.go` — thin adapter: LSP params →
  core request → core edits → `WorkspaceEdit`; typed core
  error → `InvalidParams{data.conflict}`. Duplicated
  computation deleted (no half-formed duplicate, per
  `cross-system.md`).
- LSP call-hierarchy handlers — delegate to a shared
  `internal/index` deps query so CLI and LSP share one walk.
- `cmd/mdsmith/rename.go`, `cmd/mdsmith/deps.go` — thin
  handlers (<50 lines each per `go.md`), mirroring
  `cmd/mdsmith/backlinks.go`. Neither imports `internal/lsp`.

### `mdsmith rename` (name-based contract)

```bash
mdsmith rename <file> --heading "Old Title" "New Title"
mdsmith rename <file> --link-ref oldlabel newlabel
```

Rewrites the heading / def plus every dependent edit across
the workspace in place. `--format text|json` summarizes the
files touched. Slug collision / label conflict / empty /
invalid-char fail with exit 2 and a message naming the
conflict (the CLI mirror of plan 151's `data.conflict`).

### `mdsmith deps` (dependency graph)

```bash
mdsmith deps <file>              # outgoing: includes/catalogs/builds/links
mdsmith deps <file> --incoming   # files that depend on <file>
```

`--format text|json`. Builds a transient `internal/index`
over discovered files and queries `OutgoingEdges` /
`IncomingEdges` / `BacklinksFor`.

### Feature docs

- New `docs/features/rename.md`, `docs/features/dependency-graph.md`.
- Expand `docs/features/live-diagnostics.md` so the remaining
  navigation suite is actually described (combined, per the
  scope decision).
- Add cards to `docs/features/index.md`; add
  `docs/reference/cli/rename.md` and `cli/deps.md` (catalog
  auto-regenerates).

## Tasks

1. [x] Create this plan; `mdsmith fix PLAN.md`.
2. [x] Relocate `internal/lsp/index` → `internal/index`
   (`git mv`; rewrite import paths in `internal/lsp/*.go` +
   tests + every `*.md` repo-path reference). Layer move
   only; `go build/test` + existing index/lsp tests are the
   regression gate.
3. [x] Update architecture docs (`go.md` SRP list + DIP
   arrows, `index.md` layering map) and append the plan-153
   supersession to the audit log. (`cross-system.md`
   boundaries/versioning deferred to task 8 with the CLI
   surface.)
4. [x] TDD `internal/rename` core. The link-reference
   engine and the heading engine are both lifted into
   `internal/rename` (neutral `Edit`/`Position`/`Range`,
   typed `ErrEmptyLabel` / `InvalidLabelRuneError` /
   `LabelConflictError` / `InvalidHeadingRuneError` /
   `ErrEmptyHeadingSlug` / `HeadingCollisionError`). The
   heading path takes a `Workspace` seam (incoming anchor
   edges, file list, path→bytes) so LSP can back it with
   the warm index + buffers and the CLI with a transient
   index + disk. Behavior-tested at **100%** statement
   coverage with a real index-backed workspace (no mocks);
   every production function has a dedicated unit test.
5. [ ] Refactor `internal/lsp/rename.go` to delegate to
   `internal/rename`; delete duplicated computation. Plans
   151/131 + `cmd/mdsmith/lsp_rename_test.go` stay green.
6. [ ] TDD `cmd/mdsmith/rename.go` (unit + e2e), register in
   `main.go` dispatch + `usageText`.
7. [x] TDD `cmd/mdsmith/deps.go` (unit + e2e); register
   in `main.go`. The LSP call-hierarchy and `deps` both
   consult `internal/index` for the edge graph, so the
   walk is already shared — no separate query-extraction
   refactor was needed.
8. [x] Feature + reference docs; `mdsmith fix .` to
   regenerate catalogs (CLAUDE.md, cli.md, README grid,
   CLAUDE.md include mirrors); `mdsmith check .`.
   `cross-system.md` left as-is: the CLI-surface row
   already covers subcommands generically, and "adding a
   flag/command is minor" is stated there.
9. [ ] Final gate + flip status to ✅; push. **Remaining
   before ✅: tasks 4–6 (the `internal/rename` core, the
   LSP delegation, and the `mdsmith rename` CLI).**

## Acceptance Criteria

- [x] `internal/index` exists; no production file imports
      `internal/lsp/index`; `grep -r internal/lsp/index`
      finds nothing (SRP / DIP — package answers one
      question, CLI no longer reaches the editor layer).
- [x] `internal/rename` returns plain edits and typed errors,
      imports neither `internal/lsp` nor any LSP wire type
      (DIP — surfaces depend on the core).
- [ ] `internal/lsp/rename.go` contains no slug / edit
      computation; it delegates to `internal/rename` (no
      duplicated logic across surfaces — `cross-system.md`).
- [x] Plans 131/151 LSP test suites and
      `cmd/mdsmith/lsp_rename_test.go` pass unchanged
      (Liskov — the relocation is behavior-preserving; the
      delegation refactor in task 5 must keep them green).
- [ ] `mdsmith rename f.md --heading "A" "B"` rewrites the
      heading and every workspace anchor link; `--link-ref`
      rewrites def + uses; collisions exit 2 naming the
      conflict.
- [x] `mdsmith deps f.md` and `--incoming` emit the
      dependency edges in text and json.
- [ ] CLI rename + deps contracts locked by e2e tests in
      `cmd/mdsmith/` (cross-system contract test). deps
      done (`e2e_deps_test.go`); rename pending task 6.
- [x] Every new production function has a dedicated unit
      test (`TestFoo` / `TestReceiver_Foo`). Holds for the
      shipped `deps` code; re-verify when the rename core
      lands.
- [x] `docs/features/` documents rename, the dependency
      graph, and the full navigation suite;
      `docs/features/index.md` and the CLI reference list the
      new commands.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.
- [x] `mdsmith check .` passes.

## Open Questions

- **`deps` command name.** `deps` chosen over `graph` /
  `list deps` for parity with the top-level `rename`
  subcommand and the "what does this depend on" framing.
  Easy to rename before the contract test locks if a
  reviewer prefers otherwise.

## Remaining work

Task 4 is complete:
[internal/rename](../internal/rename/rename.go) holds
the link-reference engine and
[heading.go](../internal/rename/heading.go) the
heading engine, both 100% covered. Two slices remain,
each its own green commit.

### Slice A — move the heading engine (done)

The heading half of
[internal/lsp/rename.go](../internal/lsp/rename.go)
moved into
[internal/rename/heading.go](../internal/rename/heading.go).
It covers the slug map, the anchor edits, the
ref-def edits, and the stable sort. A `Workspace`
seam feeds it the incoming edges, the file list, and
a path→bytes resolver. `rename.Heading` returns a
per-key `Edit` set. An unsafe rename returns a typed
error instead.

The LSP package still carries its own copy. Slice B
deletes that copy. Behavior tests cover same-file
anchors, cross-file anchors, the disambiguator
shift, and each guard at 100%, no mocks.

### Slice B — LSP delegates to the core

- `handleRename` resolves the cursor (unchanged), then
  calls `rename.Heading` / `rename.LinkRef`
- map the `Edit` set → `workspaceEdit`; map typed
  errors → `InvalidParams` + `renameCollisionData`
- delete the now-duplicated engine from the LSP
  package
- keep the prepare-range / cursor-disambiguation code
  (editor-only); `isValidRefDefLine` calls
  `rename.ValidRefDefBodyLines`
- regression gate, byte-for-byte: the plan-131/151
  suites in [internal/lsp](../internal/lsp/) plus
  [cmd/mdsmith/lsp_rename_test.go](../cmd/mdsmith/lsp_rename_test.go)
- the neutral `Edit` is line + UTF-16 char, the same
  shape as the LSP `textEdit`, so the adapter is a
  field copy — the wire output cannot drift

### Slice C — the `mdsmith rename` CLI

Add `cmd/mdsmith/rename.go`, name-based:

```bash
mdsmith rename <file> --heading "Old" "New"
mdsmith rename <file> --link-ref oldlabel newlabel
```

- `--heading` finds the heading line whose text is
  `Old`; `--link-ref` normalizes `oldlabel`
- build a `Workspace` over a transient
  `internal/index` + disk, mirroring
  [cmd/mdsmith/deps.go](../cmd/mdsmith/deps.go)
- apply each `Edit`: per line, convert the UTF-16
  range back to bytes, then splice
- register `rename` in
  [cmd/mdsmith/main.go](../cmd/mdsmith/main.go)
  dispatch + usage text
- exit codes: `0` rewritten, `1` no match, `2` error
  or conflict
- unit + e2e tests, 100% coverage

Then the task-8 follow-ups:

- add a CLI section to
  [docs/features/rename.md](../docs/features/rename.md)
- add a `docs/reference/cli/rename.md` page
- run `mdsmith fix .` to regenerate the catalogs
- flip this plan's status to ✅

## ...

<?allow-empty-section?>
