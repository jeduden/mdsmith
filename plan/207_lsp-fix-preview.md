---
id: 207
title: LSP fix preview via ChangeAnnotation
status: "🔲"
summary: >-
  Opt-in `mdsmith.previewFix` setting that attaches an
  LSP `ChangeAnnotation` with `needsConfirmation:
  true` to every quick-fix and `source.fixAll.mdsmith`
  WorkspaceEdit, so VS Code opens its Refactor
  Preview pane with a diff before applying.
model: sonnet
depends-on: []
---
# LSP fix preview via ChangeAnnotation

## Goal

Let the user opt into a preview pane before any
auto-fix lands. With `mdsmith.previewFix` on,
every code action from
[`mdsmith lsp`](../internal/lsp/server.go) carries
a `ChangeAnnotation` flagged
`needsConfirmation: true`. VS Code routes the
edit to its Refactor Preview. The diff renders
there. The user accepts or rejects per fix.

## Background

Today the LSP server returns a `WorkspaceEdit`
inline on each `codeAction` result. VS Code
applies it the moment the user clicks the
lightbulb. The same happens on save when
`editor.codeActionsOnSave` is set. No preview
fires unless the user picks "Refactor Preview"
by hand from the lightbulb menu.

LSP 3.16 added `ChangeAnnotation` and a sibling
`AnnotatedTextEdit` shape. The edit carries an
`annotationId`. The id points at the workspace
edit's `changeAnnotations` map. Mark the
annotation `needsConfirmation: true` and VS Code
routes through Refactor Preview. The same
machinery drives rename preview and TypeScript's
"organize imports" preview.

The CLI side shipped the matching gate in
[plan 137](137_fix-dry-run.md) — `mdsmith fix
--dry-run`. This plan brings "see before write"
to the editor. Every LSP client that advertises
`changeAnnotationSupport` benefits, not just VS
Code.

## Non-Goals

- Inline ghost-text preview (Copilot Edits / Cursor
  style). That needs proposed VS Code APIs and is
  editor-only, not LSP. Out of scope here.
- A separate "Preview" code action alongside the
  apply-immediately one. The annotation is attached
  to the existing actions; the setting toggles
  preview wholesale, mirroring how
  `mdsmith.fixOnSave` toggles save-time fix.
- Diff rendering inside the server. VS Code computes
  the diff from before/after; we just hand it the
  annotated edit.
- Per-rule opt-out. The setting is on or off; users
  who want to skip specific rules disable them in
  `.mdsmith.yml`.

## Design

### Setting

Add `mdsmith.previewFix` (boolean, default
`false`). Declare it on
[the extension's `package.json`](../editors/vscode/package.json).
Mirror it on the `clientSettings` shape in
[`internal/lsp/server.go`](../internal/lsp/server.go).
The server reads it on the same
`workspace/configuration` round-trip as
`mdsmith.config` and `mdsmith.run`.

When `previewFix` is `false`, the server returns
today's `WorkspaceEdit` shape. No behavioral
change for the default install.

### Capability negotiation

On `initialize`, read the client capability.
The flag lives at
`workspace.workspaceEdit.changeAnnotationSupport`.
Missing? Then use the legacy `changes` form.
This holds even when `previewFix` is on. Log
the fallback once to the output channel. The
log line reads: "client does not support
changeAnnotationSupport".

### Edit shape

`AnnotatedTextEdit` lives inside
`WorkspaceEdit.documentChanges`. The spec bans it
under the legacy `changes` map. With preview on,
the edit looks like:

```jsonc
{
  "documentChanges": [{
    "textDocument": { "uri": "...", "version": null },
    "edits": [{
      "range": { ... },
      "newText": "...",
      "annotationId": "mdsmith-fix-MDS001"
    }]
  }],
  "changeAnnotations": {
    "mdsmith-fix-MDS001": {
      "label": "Fix all MDS001 with mdsmith",
      "description": "Preview before applying",
      "needsConfirmation": true
    }
  }
}
```

The `source.fixAll.mdsmith` action gets one
annotation. Its label reads
`"Fix all mdsmith issues"`. Per-rule quick-fix
actions get one annotation each, keyed
`mdsmith-fix-<rule>`. The preview pane then groups
the changes by rule.

### Document version

`OptionalVersionedTextDocumentIdentifier.version`
is `null`. That means "match whatever buffer the
client has". The server does not track per-edit
buffer versions today. `null` keeps the contract
the same as the legacy `changes`-map form.

### Default off

`mdsmith.previewFix` defaults to `false`. A
confirmation pane on every save would be hostile.
Users opt in explicitly. The doc page calls out
the matrix:

- `previewFix: true` + `fixOnSave: true` → every
  save opens a preview pane.
- `previewFix: true` + `fixOnSave: false` →
  preview fires only when the user clicks the
  lightbulb.

## Tasks

1. Extend `clientSettings` in
   [`internal/lsp/server.go`](../internal/lsp/server.go)
   with `PreviewFix *bool` and wire the read in
   `fetchClientSettings`.
2. Capture
   `workspace.workspaceEdit.changeAnnotationSupport`
   from `initialize` params; store on the server.
3. Grow the protocol types in
   [`internal/lsp/protocol.go`](../internal/lsp/protocol.go).
   New fields on `workspaceEdit`:
   `documentChanges` and `changeAnnotations`.
   New types: `annotatedTextEdit`,
   `textDocumentEdit`, `changeAnnotation`. Keep
   `changes` for the legacy path.
4. Refactor `fullFileEdit` (and any other edit
   builder) to take a "preview mode" decision and
   emit either the legacy `changes`-map shape or
   the annotated `documentChanges` shape. Mode is
   `preview ∧ clientSupportsAnnotations`.
5. Per-rule annotation IDs in `computeCodeActions`:
   `mdsmith-fix-<rule>` for quick fixes,
   `mdsmith-fix-all` for `source.fixAll.mdsmith`.
   Label uses the existing `quickFixTitle`.
6. Add the `mdsmith.previewFix` setting to
   [`editors/vscode/package.json`](../editors/vscode/package.json)
   contributions with description and default
   `false`.
7. Document the setting in
   [`docs/guides/editors/vscode.md`](../docs/guides/editors/vscode.md)
   (Settings table + a short "Preview before
   applying" subsection).
8. Document the LSP behavior in
   [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
   — capability requirement and the wire shape.
9. Tests:

  - server unit: when client capability is
    absent, edits stay in the legacy `changes`
    form regardless of `previewFix`.
  - server unit: when capability + setting are
    both on, `source.fixAll.mdsmith` returns a
    `documentChanges` edit with a single
    `mdsmith-fix-all` annotation
    (`needsConfirmation: true`).
  - server unit: per-rule quick fix returns one
    annotation per rule, IDs `mdsmith-fix-<rule>`.
  - server unit: setting off → edits identical
    to today's bytes (snapshot regression).
  - extension unit: `package.json` exposes
    `mdsmith.previewFix` boolean with default
    `false`.

## Acceptance Criteria

- [ ] `mdsmith.previewFix=false` (the default)
      leaves the LSP `WorkspaceEdit` byte-identical
      to today's output for both `quickfix` and
      `source.fixAll.mdsmith`.
- [ ] `mdsmith.previewFix=true` + a client that
      advertises `changeAnnotationSupport`
      produces an edit using `documentChanges`
      with `changeAnnotations[*].needsConfirmation
      = true`.
- [ ] Without `changeAnnotationSupport` the server
      falls back to the legacy `changes` shape
      and logs the fallback once per session.
- [ ] Quick-fix actions carry one annotation per
      rule (`mdsmith-fix-<rule>`);
      `source.fixAll.mdsmith` carries
      `mdsmith-fix-all`.
- [ ] [`docs/guides/editors/vscode.md`](../docs/guides/editors/vscode.md)
      documents the setting and the
      `previewFix` × `fixOnSave` interaction.
- [ ] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      documents the new wire shape and the
      capability gate.
- [ ] All tests pass: `go test ./...` and
      `bun test` in `editors/vscode`.
- [ ] `go tool golangci-lint run` reports no
      issues.
