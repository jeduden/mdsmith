---
command: lsp
summary: Run a Language Server Protocol server on stdio for editor integrations.
---
# `mdsmith lsp`

Run an LSP server that speaks the Language Server Protocol over
stdio. The server reuses the same lint and fix pipelines as
`check` and `fix`, surfaces diagnostics, and exposes per-rule
quick fixes plus a whole-file `source.fixAll.mdsmith` action.

```text
mdsmith lsp [--stdio]
```

The subcommand is designed to be spawned by an LSP client (VS
Code, Neovim, Helix, JetBrains LSP plugin), not run
interactively. It reads JSON-RPC frames on stdin and writes
responses and notifications on stdout.

`--stdio` is accepted as a no-op for compatibility with LSP
clients (notably `vscode-languageclient`) that append the flag
whenever the client selects stdio transport. The server always
uses stdio either way.

## Capabilities advertised

| Capability                        | Behavior                                                                           |
|-----------------------------------|------------------------------------------------------------------------------------|
| `textDocumentSync = Full`         | Full-document sync; lint trigger gated by `mdsmith.run`                            |
| `publishDiagnostics`              | One push after each lint                                                           |
| `codeActionProvider`              | `quickfix` per fixable diagnostic, `source.fixAll.mdsmith`                         |
| `documentSymbolProvider`          | Hierarchical outline (headings, link refs, front matter, directives)               |
| `definitionProvider`              | Jump-to-definition for anchor / file / ref-style links and directive arguments     |
| `implementationProvider`          | Multi-target jump for `kind:` values and headings (every link target)              |
| `referencesProvider`              | Workspace links pointing at the symbol under the cursor                            |
| `workspaceSymbolProvider`         | Substring search across headings, link refs, front-matter `title:`, and kind names |
| `callHierarchyProvider`           | File-level call graph over `<?include?>`, `<?catalog?>`, `<?build?>`, and links    |
| `workspace/didChangeWatchedFiles` | Re-lint open buffers on `.mdsmith.yml` change; index refresh on Markdown changes   |

`mdsmith.run` controls when the server actually re-lints:

- `onSave` (default): lint on `didOpen`, `didSave`, and config
  changes. `didChange` events update the buffer but do not trigger a
  lint pass.
- `onType`: lint on every `didChange` (debounced 200 ms) plus the
  same triggers as `onSave`.
- `off`: never lint automatically. Code actions still work when
  invoked explicitly.

## Diagnostic mapping

LSP `Diagnostic` fields map from the same JSON shape `check`
prints:

| mdsmith          | LSP                                                                     |
|------------------|-------------------------------------------------------------------------|
| `rule` + `name`  | `code` (e.g. `MDS001`); `source = mdsmith`                              |
| `severity`       | `severity` (error → 1, warning → 2)                                     |
| `line`, `column` | `range.start`; end is the line's UTF-16 length (squiggle → end-of-line) |
| `message`        | `message`                                                               |
| rule name        | `data.rule` (echoed back on codeAction)                                 |

## Code actions

- **`quickfix`** — one per fixable diagnostic. Each
  edit replaces the whole document with the output of
  running the single rule, so it covers every
  occurrence of that rule (the action title reads
  "Fix all `<rule>` with mdsmith"). Within one
  request all quick-fix actions for the same rule
  share one `WorkspaceEdit`; the fix is run once
  regardless of how many diagnostics carry that
  rule. Generated-section rules (catalog, toc,
  include) regenerate the section in their fix; the
  action surfaces normally and the title
  ("Fix all `<rule>` with mdsmith") is explicit
  about the whole-file scope.
- **`source.fixAll.mdsmith`** — runs `mdsmith fix` on the
  current buffer; produces the same bytes the on-disk fixer
  would write.

## Symbol navigation

The server indexes the workspace into a symbol graph. The
graph is built lazily on the first symbol-navigation
request and is kept in sync via:

- `didOpen` / `didChange` re-parse the open buffer
  and swap its slice of the index.
- `**/*.md` watcher events refresh one file from disk
  when it changes outside any open buffer.
- `.mdsmith.yml` changes invalidate the whole index
  because kind / ignore globs may shift scope.

### Symbol kinds

| Concept                   | LSP `SymbolKind` | Container                 |
|---------------------------|------------------|---------------------------|
| Heading (H1–H6)           | `String` (15)    | parent heading            |
| Link-reference definition | `Key` (20)       | file                      |
| Front-matter field        | `Property` (7)   | file                      |
| Directive (`<?name … ?>`) | `Event` (24)     | enclosing heading or file |

Headings drive the outline; the others hang off the
synthetic file-root entry. The cross-document key is
`(file, anchor)` for headings (slug from
`mdtext.CollectTOCItems`) and `(file, label)` for link
refs.

### Definition and implementation

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

### References

| Cursor on…                          | References returned                              |
|-------------------------------------|--------------------------------------------------|
| Heading                             | every workspace link to `(file, anchor)`         |
| `[label]: url` definition           | every `[text][label]` and shortcut in the file   |
| File line 1                         | every link target with this path (no anchor)     |
| `kind:` value                       | every file with that kind assignment             |
| Directive arg (`file:` / `source:`) | every directive whose `file:` / `source:` = this |

`includeDeclaration: false` excludes the heading or
definition itself.

### Workspace symbol

The query is a case-insensitive substring. It matches
heading text, link-ref labels, front-matter `title:`,
and kind names. The relative path goes in
`containerName`.

### Call hierarchy

A Markdown file is the unit of "function"; an outbound
reference is a "call". `incomingCalls` answers "who
depends on this runbook?", `outgoingCalls` answers
"what does this overview embed?".

`prepareCallHierarchy` accepts three cursor positions:

- File root → the item is the file.
- Heading line → the item is that heading section.
- Directive arg → the item is the target file.

`incomingCalls` returns every edge into the item, with
sources from cross-file links, `<?include?>`,
`<?catalog?>` matches, and `<?build?>`. Each entry
carries the source file and the reference line.
`outgoingCalls` returns every edge out of the item;
catalog matches collapse to one entry per directive
(expansion would inflate large globs into noise).

## Configuration discovery

The server uses workspace-wide discovery. It starts
at the workspace root supplied at `initialize`
(`rootUri` or the first `workspaceFolders` entry)
and walks upward until it finds a `.mdsmith.yml` or
hits a `.git` boundary — the same walk `mdsmith
check` uses from its CWD. Every open buffer shares
the resolved config; the server does not re-discover
per file.

Clients override the walk with `mdsmith.config`,
which the server pulls via `workspace/configuration`.
Edits to `.mdsmith.yml` invalidate the cached
config. The server then re-lints every open document
immediately.

## Example

For client setup and troubleshooting see the
[VS Code guide](../../guides/editors/vscode.md). Other LSP
clients can spawn the binary directly:

```bash
mdsmith lsp
```

## Performance

The squiggle-update path is benchmarked under
`internal/lsp/`. Plan 121 sets a p95 budget of 150 ms on a
1 000-line buffer and 500 ms on a 5 000-line buffer. Run the
benchmark locally with:

```bash
go test -run=^$ -bench=. ./internal/lsp/...
```

## Exit codes

| Code | Meaning                    |
|------|----------------------------|
| 0    | Server exited cleanly      |
| 2    | Runtime or transport error |

## See also

- [`mdsmith check`](check.md) — the CLI surface that the
  server reuses
- [`mdsmith fix`](fix.md) — the fix pipeline behind both
  code actions
- [VS Code guide](../../guides/editors/vscode.md) — install,
  settings, troubleshooting
