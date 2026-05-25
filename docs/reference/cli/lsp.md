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

`--stdio` is accepted as a no-op for clients (notably
`vscode-languageclient`) that always append it when selecting
stdio transport. The server uses stdio either way.

## Capabilities advertised

| Capability                        | Behavior                                                                           |
| --------------------------------- | ---------------------------------------------------------------------------------- |
| `textDocumentSync = Full`         | Full-document sync; lint trigger gated by `mdsmith.run`                            |
| `publishDiagnostics`              | One push after each lint                                                           |
| `codeActionProvider`              | `quickfix` per fixable diagnostic, `source.fixAll.mdsmith`                         |
| `hoverProvider`                   | Rule docs on hover over a diagnostic; directive docs on hover inside `<?…?>`       |
| `documentSymbolProvider`          | Hierarchical outline (headings, link refs, front matter, directives)               |
| `definitionProvider`              | Jump-to-definition for anchor / file / ref-style links and directive arguments     |
| `implementationProvider`          | Multi-target jump for `kind:` values and headings (every link target)              |
| `referencesProvider`              | Workspace links pointing at the symbol under the cursor                            |
| `workspaceSymbolProvider`         | Substring search across headings, link refs, front-matter `title:`, and kind names |
| `callHierarchyProvider`           | File-level call graph over `<?include?>`, `<?catalog?>`, `<?build?>`, and links    |
| `completionProvider`              | Heading anchors, link-ref labels, kind names, and directive file paths             |
| `renameProvider`                  | Heading + link-reference label renames, with `prepareProvider: true`               |
| `workspace/didChangeWatchedFiles` | Re-lint open buffers on `.mdsmith.yml` change; index refresh on Markdown changes   |

`mdsmith.run` controls when the server actually re-lints:

- `onSave` (default): lint on `didOpen`, `didSave`, and config
  changes. `didChange` events update the buffer but do not trigger a
  lint pass.
- `onType`: lint on every `didChange` (debounced 200 ms) plus the
  same triggers as `onSave`.
- `off`: never lint automatically. Code actions still work when
  invoked explicitly.

## Hover

`textDocument/hover` resolves in two passes:

1. **Diagnostic-first.** If the cursor falls inside an active diagnostic
   range, the server returns a `MarkupContent` (kind `markdown`). The
   body begins with the diagnostic message followed by the rule's full
   help text — the same text `mdsmith help rule <id>` prints.

2. **Directive fallback.** If no diagnostic covers the cursor, the
   server checks whether the cursor is inside a `<?directive …?>`
   block. If so, it returns the directive's guide page from
   `docs/guides/directives/`. The documented directives are
   `catalog`, `include`, `build`, `allow-empty-section`, and
   `require`.

If neither pass finds a match, the server returns `null` (no hover).
Each hover response includes a `range` field set to the matched span
— the diagnostic range or the full directive block range — so clients
can anchor the popup to the right span.

`mdsmith/rulePatterns` returns rule maintainability metadata; hover
adds "Suggested remediation" only when `for-diagnostic: true`.

## Diagnostic mapping

LSP `Diagnostic` fields map from the same JSON shape `check`
prints:

| mdsmith          | LSP                                                                     |
| ---------------- | ----------------------------------------------------------------------- |
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

### Fix preview (ChangeAnnotation)

Set `mdsmith.previewFix: true` to open Refactor Preview before
any fix lands. Both capabilities below must appear in
`initialize`:

- `workspace.workspaceEdit.documentChanges`
- `workspace.workspaceEdit.changeAnnotationSupport`

When both are present, edits use `AnnotatedTextEdit`
(LSP 3.16) with `needsConfirmation: true`. The
`edits` slice carries one entry per line-aligned
diff hunk, not one whole-file replacement (which
Refactor Preview would render as "old file → new
file" with no visible delta). All hunks share an
`annotationId`.

Each quick fix carries the id
`mdsmith-fix-<rule-name>`. The fix-all action uses
`mdsmith-fix-all`. Drop either capability and the
server emits the legacy `changes` map. A warning
goes to `window/logMessage` once per session.

## Symbol navigation

The server indexes the workspace into a symbol graph. The
graph is built lazily on the first symbol-navigation
request and is kept in sync via:

- `didOpen` / `didChange` re-parse the open buffer
  and swap its slice of the index.
- `**/*.md` watcher events refresh one file from disk
  when it changes outside any open buffer.
- `.mdsmith.yml` changes invalidate the whole index
  because `ignore:`, `kind-assignment:`, and
  `follow-symlinks:` all shift what the index sees.
  Open buffers bypass `ignore:` (the user editing a
  file always wants it visible).

### Symbol kinds

| Concept                   | LSP `SymbolKind` | Container                 |
| ------------------------- | ---------------- | ------------------------- |
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
| ------------------------------ | ---------------------------- | -------------------------- |
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
| ----------------------------------- | ------------------------------------------------ |
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

## Completion

The server handles `textDocument/completion` and advertises:

```jsonc
"completionProvider": {
  "triggerCharacters": ["#", "[", ":", "/", "\""],
  "resolveProvider": false
}
```

Completion items are fully computed in one pass from the workspace symbol
index (`resolveProvider: false`). Items are returned sorted with same-file
matches first for anchor completion.

### Supported contexts

| Cursor on…                         | Items returned                  | `kind`       |
| ---------------------------------- | ------------------------------- | ------------ |
| `[text](#prefix`                   | Heading anchors in current file | `Reference`  |
| `[text](./other.md#prefix`         | Heading anchors in `other.md`   | `Reference`  |
| `[text][prefix`                    | Link-ref labels in current file | `Reference`  |
| Front-matter `kind: prefix`        | Kind names from `.mdsmith.yml`  | `EnumMember` |
| Front-matter `kinds:` list item    | Kind names from `.mdsmith.yml`  | `EnumMember` |
| `<?include file: "prefix"?>` arg   | Workspace Markdown paths        | `File`       |
| `<?build source: "prefix"?>` arg   | Workspace Markdown paths        | `File`       |
| `<?catalog glob: "prefix"?>` entry | Workspace Markdown paths        | `File`       |
| Any other position                 | Empty list (no error)           | —            |

The `detail` field carries the source file path for headings and
link-ref labels, and `.mdsmith.yml` for kind names.

Duplicate-slug anchors (`foo`, `foo-1`, `foo-2`, …) are each returned
as separate items.

Directive-arg paths are relative to the open buffer's directory.
This matches how `ResolveRelTarget` resolves them at lint time.
Both `.md` and `.markdown` files appear as candidates.

Image links (`![alt](#…`) do not trigger anchor completion.
Completion inside fenced or indented code blocks returns an empty list.

### Rename

`prepareRename` returns the text range for ATX heading text
(without `#`s), setext heading text lines, `[label]: url`
defs, the trailing `[…]` of a full reference, and the
leading `[…]` of a shortcut or collapsed reference. The
placeholder pre-fills the popup; other positions return
`null`.

Heading rename rewrites the heading and every workspace
anchor link to its slug. Duplicate-name disambiguator shifts
emit follow-up edits. Link-ref rename rewrites the
`[label]: url` def plus every same-file use. `InvalidParams`
fires on a new duplicate base slug, a colliding def, an
empty slug, or a `[` / `]` / newline in a label. The
error's `data.conflict` names the colliding symbol.

## Configuration discovery

Discovery is workspace-wide. Starting at the workspace root
from `initialize`, the server walks up until it finds a
`.mdsmith.yml` or hits `.git`. Every open buffer shares the
resolved config. Set `mdsmith.config` to override the walk.
Edits to `.mdsmith.yml` re-lint all open documents immediately.

## Performance

Run `go test -run=^$ -bench=. ./internal/lsp/...` to reproduce
the p95 latency benchmarks (150 ms on 1 000-line, 500 ms on
5 000-line buffers).

## Exit codes

| Code | Meaning                    |
| ---- | -------------------------- |
| 0    | Server exited cleanly      |
| 2    | Runtime or transport error |

## See also

- [`mdsmith check`](check.md) — the CLI surface that the server reuses
- [`mdsmith fix`](fix.md) — the fix pipeline behind both code actions
- [VS Code guide](../../guides/editors/vscode.md) — install,
  settings, troubleshooting
