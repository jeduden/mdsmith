---
id: 131
title: LSP symbol navigation for agents (Claude)
status: "✅"
model: opus
summary: >-
  Extend `mdsmith lsp` with the document-symbol,
  definition, implementation, references,
  workspace-symbol, and call-hierarchy methods so an
  LSP-aware agent (Claude's LSP tool, Neovim, Helix)
  can navigate Markdown by heading outline, anchor
  and file links, and the include/catalog graph.
---
# LSP symbol navigation for agents (Claude)

## Goal

Let an LSP client navigate Markdown like code: list
the file outline, jump from links to targets,
enumerate references to a heading, search by name,
and walk the include/catalog graph as a call
hierarchy. The server reuses the existing AST and
config plumbing.

## Background

Plan 121 shipped `mdsmith lsp` with diagnostics and
code actions. Plan 122 adds hover for rule and
directive docs. Neither covers symbol navigation.

Claude's LSP tool exposes nine methods:

| LSP method                          | Agent intent                    |
|-------------------------------------|---------------------------------|
| `textDocument/documentSymbol`       | List symbols in this file       |
| `textDocument/definition`           | Where is this defined?          |
| `textDocument/implementation`       | Where is the concrete behavior? |
| `textDocument/hover`                | What is this?                   |
| `textDocument/references`           | Who uses this?                  |
| `workspace/symbol`                  | Find a symbol by name           |
| `textDocument/prepareCallHierarchy` | Anchor a call-graph view        |
| `callHierarchy/incomingCalls`       | Who calls this?                 |
| `callHierarchy/outgoingCalls`       | What does this call?            |

mdsmith already understands the edges these methods
need: headings, anchor and file links, link-ref
defs, front-matter `kind:`, and directive
arguments. The
[MDS027 rule](../internal/rules/MDS027-cross-file-reference-integrity/README.md)
walks every link target. Catalog expands globs.

## Non-Goals

- New transports. Stdio stays.
- Rename refactoring. Heading rewrites need anchor
  fixups across the workspace; separate plan.
- Type hierarchy. No Markdown analogue.
- Code lens, inlay hints, semantic tokens.
- Indexing outside the workspace root. The index
  obeys the existing
  [`internal/discovery`](../internal/discovery)
  walk.

## Design

### Symbol model

A symbol is one of four things, with a `SymbolKind`
chosen so picker UIs bucket each sensibly:

| Concept                   | `SymbolKind`   | Container                 |
|---------------------------|----------------|---------------------------|
| Heading (H1–H6)           | `String` (15)  | parent heading            |
| Link-reference definition | `Key` (20)     | file                      |
| Front-matter field        | `Property` (7) | file                      |
| Directive (`<?name … ?>`) | `Event` (24)   | enclosing heading or file |

Headings drive the outline; the others sit flat.
The cross-document key is `(file, anchor)` for
headings (slug from
[`mdtext.CollectTOCItems`](../internal/mdtext/mdtext.go))
and `(file, label)` for link refs.

### Workspace index

A new package `internal/index` holds the
symbol graph. It stores headings, link-reference
defs, front-matter top-level keys, directives, and
both directions of the reference edges (link
targets, include / catalog / build targets).

Build is lazy on the first symbol request. Update
is incremental:

- `didOpen` / `didChange` / `didSave` re-parses one
  buffer and swaps its slice.
- `**/*.md` watcher (added here; today's watcher
  covers only `.mdsmith.yml`) invalidates one file.
- `.mdsmith.yml` change rebuilds the whole index
  because kind / ignore globs may shift scope.

The index calls
[`lint.ParseFile`](../internal/lint/file.go) once
per file. Existing visitors cover headings and
link targets. A new visitor captures
`*ast.LinkReferenceDefinition` and front-matter keys.
Memory at 10 000 files is ~300K entries, well
under plan 121's 512 MB `GOMEMLIMIT`.

### `textDocument/documentSymbol`

Returns a `DocumentSymbol[]` tree rooted at H1s.
Each heading carries name, anchor in `detail`,
range from heading to next sibling, and children.
Front-matter keys hang off a synthetic top-of-file
symbol. Directives become children of their
enclosing heading.

Capability: `documentSymbolProvider = true`.

### `textDocument/definition` and `…/implementation`

Both share one `resolveTarget(uri, position)` core.
`Implementation` returns multi-target sets where
`Definition` returns one.

| Cursor on…                     | `Definition`                 | `Implementation` adds      |
|--------------------------------|------------------------------|----------------------------|
| `[text](#anchor)`              | heading in this file         | —                          |
| `[text](./other.md)`           | line 1 of `other.md`         | —                          |
| `[text](./other.md#anchor)`    | heading in `other.md`        | —                          |
| `[text][label]`                | matching `[label]: url`      | —                          |
| `<?include file: "x.md"?>` arg | `x.md` line 1                | —                          |
| `<?build source: "x.md"?>` arg | `x.md` line 1                | —                          |
| `kind:` value in front matter  | kind block in `.mdsmith.yml` | every file with that kind  |
| Heading line                   | the heading                  | every link target matching |

A small helper `internal/index/locate.go` maps
a position to an AST node and a token tag (heading,
anchorLink, fileLink, refUse, refDef, directiveArg,
frontMatterKey, frontMatterValue). One unit test
per token tag.

Capabilities: `definitionProvider = true`,
`implementationProvider = true`.

### `textDocument/references`

| Cursor on…                | References returned                              |
|---------------------------|--------------------------------------------------|
| Heading                   | every workspace link to `(file, anchor)`         |
| `[label]: url` definition | every `[text][label]` and shortcut in the file   |
| File line 1               | every link target with this path (no anchor)     |
| `kind:` value             | every file with that kind assignment             |
| Directive block           | every directive whose `file:` / `source:` = this |

`includeDeclaration: false` excludes the heading or
definition itself. Capability:
`referencesProvider = true`.

### `workspace/symbol`

The query is a case-insensitive substring. It
matches heading text, link-ref labels, kind names,
and front-matter `title:`. The relative path goes
in `containerName`. Capability:
`workspaceSymbolProvider = true`.

### Call hierarchy

A Markdown file is the unit of "function"; an
outbound reference is a "call". This fits doc
workflows: `incomingCalls` answers "who depends on
this runbook?", `outgoingCalls` answers "what does
this overview embed?".

`prepareCallHierarchy` accepts three cursor
positions. On line 1, the item is the file. On a
heading, the item is that heading section and
calls are scoped to its range. On a directive arg,
the item is the target file.

`incomingCalls` returns every edge into the item.
Sources include cross-file links, `<?include?>`,
`<?catalog?>` matches, and `<?build?>`. Each entry
carries the source file and the reference line.
`outgoingCalls` returns every edge out of the
item. Catalog matches reuse the cached glob
expansions for MDS019.

Capability: `callHierarchyProvider = true`.

### Position and performance

Ranges follow the UTF-16 column convention plan
121 set; the
[`utf16Length`](../internal/lsp/diagnostics.go)
helper extends unchanged. Budgets: cold build
under 1 s on 1 000 files, incremental update under
20 ms per `didChange`. A new
`internal/index/bench_test.go` measures both
on synthetic 100 / 1 000 / 10 000-file workspaces;
the plan 121 benchmark CI step picks it up.

### Backwards compatibility

Diagnostics and code-action behavior are
unchanged. New capabilities are additive. A client
that ignores them sees the post-plan-121 server.

## Tasks

1. Add `internal/index` with the symbol graph
   types and `Build` / `Update` / `Remove` entry
   points. Cover heading collection, link-ref defs,
   front-matter keys, and directive parsing in unit
   tests. Reuse `mdtext.CollectTOCItems`.
2. Add the inbound / outbound edge tables. Sources:
   anchor links, file links, `<?include?>`,
   `<?catalog?>`, `<?build?>`. Reuse
   [`lint/pi_parser.go`](../internal/lint/pi_parser.go).
3. Add `internal/index/locate.go` mapping a
   document URI plus position to an AST node and
   token tag. One test per tag.
4. Wire the index into the server. Build lazily on
   first symbol request; update on document
   events; rebuild on `.mdsmith.yml` change;
   invalidate on `**/*.md` watcher events. Extend
   [`registerWatchers`](../internal/lsp/server.go).
5. Implement `textDocument/documentSymbol` and add
   the capability. Integration test against a
   fixture with H1/H2/H3 headings, directives, and
   link refs.
6. Implement `textDocument/definition` and
   `textDocument/implementation`. Cover every row
   in the design table.
7. Implement `textDocument/references`. Cover the
   five rows; verify `includeDeclaration`.
8. Implement `workspace/symbol`. Cover heading,
   kind, and `title:` matches.
9. Implement `prepareCallHierarchy`,
   `incomingCalls`, `outgoingCalls`. Cover file /
   heading / directive prepare paths and round-
   trip on a three-file include chain.
10. Add the bench file and the budget thresholds.
    The CI step from plan 121 covers it.
11. Extend
    [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
    with a "Symbol navigation" section, the
    symbol-kind table, and call-hierarchy
    semantics.
12. Update
    [the VS Code guide](../docs/guides/editors/vscode.md)
    with a short "Outline and Go to Definition"
    note.
13. Add an end-to-end test in
    [`cmd/mdsmith`](../cmd/mdsmith) driving
    `initialize` → `didOpen` → `documentSymbol` →
    `definition` → `references` →
    `prepareCallHierarchy` → `incomingCalls` →
    `shutdown` → `exit`.

## Acceptance Criteria

- [x] `documentSymbol` returns a hierarchical
      outline whose nesting matches heading levels.
- [x] `definition` jumps to a heading from an
      anchor link, to line 1 of a file from a
      relative link, and to the matching reference
      def from `[text][ref]`.
- [x] `implementation` on a `kind:` value returns
      one location per file assigned that kind.
- [x] `references` on a heading returns every
      workspace anchor link to it;
      `includeDeclaration: false` excludes the
      heading itself.
- [x] `workspace/symbol` substring queries match
      headings, link-ref labels, front-matter
      titles, and kind names.
- [x] `prepareCallHierarchy` on a file returns one
      item; `incomingCalls` lists files that
      include / link to it; `outgoingCalls` lists
      files it includes / links to.
- [x] Cold-build benchmark reports under 1 s on
      1 000 files; incremental-update under 20 ms
      per `didChange`. Invocation:
      `go test -run=^$ -bench=. ./internal/lsp/...`
- [x] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      lists every new capability and the symbol-
      kind table.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes including the new
      docs and the updated `PLAN.md` catalog.

## Open Questions

- **Catalog glob expansion in `incomingCalls`.** A
  glob like `**/*.md` would inflate result lists.
  The first pass collapses each catalog block to
  one entry. Add an `expandCatalog` flag later if
  needed.
- **`findReferences` across include boundaries.**
  A heading in an included file is reachable via
  both files. The first pass reports both; flag
  if noisy.
