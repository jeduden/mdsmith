---
id: 207
title: LSP fix preview via ChangeAnnotation
status: "✅"
summary: >-
  Opt-in `mdsmith.previewFix` setting that attaches
  an LSP `ChangeAnnotation` flagged
  `needsConfirmation: true` to the
  `source.fixAll.mdsmith` (fix-on-save) WorkspaceEdit,
  so VS Code opens its Refactor Preview pane with a
  diff before applying. Interactive quick fixes apply
  immediately (see the Revision note).
model: sonnet
depends-on: []
---
# LSP fix preview via ChangeAnnotation

## Revision

This was later narrowed. Only the fix-all action
(`source.fixAll.mdsmith`, run on save) asks for a preview
now. Lightbulb quick fixes apply right away. A quick fix is
the one fix you just chose, so there is nothing to confirm.

A forced preview hid the edit in a pane whose Apply button
is easy to miss, and a second click hit the open preview.
The lightbulb still has its own Preview (the chevron, or
Ctrl+Enter). See
[`internal/lsp/server.go`](../internal/lsp/server.go).

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

On `initialize`, read two client capability flags:

- `workspace.workspaceEdit.documentChanges` — the
  client honors the `documentChanges` array shape
  the annotated edit uses.
- `workspace.workspaceEdit.changeAnnotationSupport`
  — the client honors `ChangeAnnotation` and
  `needsConfirmation`.

Both must be true. If either flag is missing or
`false`, fall back to the legacy `changes` form
even when `previewFix` is on. Log the fallback
once to the output channel. The log line names
the missing capability: "client does not
support documentChanges" or "client does not
support changeAnnotationSupport".

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
      "annotationId": "mdsmith-fix-no-trailing-spaces"
    }]
  }],
  "changeAnnotations": {
    "mdsmith-fix-no-trailing-spaces": {
      "label": "Fix all no-trailing-spaces with mdsmith",
      "description": "Preview before applying",
      "needsConfirmation": true
    }
  }
}
```

Today's `workspaceEdit.Changes` field in
[`internal/lsp/protocol.go`](../internal/lsp/protocol.go)
lacks `omitempty`. A struct that sets only
`DocumentChanges` would still emit
`"changes": null`. The annotated path must omit
`changes`. The spec says clients ignore `changes`
when `documentChanges` is set; sending both is
undefined. So switch the tag to `changes,omitempty`
(or use a pointer). The legacy path then keeps
`"changes": {…}`; the annotated path emits only
`documentChanges` and `changeAnnotations`.

The `source.fixAll.mdsmith` action gets one
annotation. Its label reads
`"Fix all mdsmith issues"`. Per-rule quick-fix
actions get one annotation each. The id is
`mdsmith-fix-<rule-name>` — using the
`d.Data.RuleName` value (e.g.
`no-trailing-spaces`), not the rule code
(`MDS001`). The label reuses `quickFixTitle(rule)`
as-is, which the existing code already calls with
the same rule-name string. The preview pane then
groups the changes by rule.

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

1. [x] Extend `clientSettings` in
   [`internal/lsp/server.go`](../internal/lsp/server.go)
   with `PreviewFix *bool` and wire the read in
   `fetchClientSettings`.
2. [x] Capture both
   `workspace.workspaceEdit.documentChanges` and
   `workspace.workspaceEdit.changeAnnotationSupport`
   from `initialize` params; store on the server.
   The annotated path requires both to be true.
3. [x] Grow the protocol types in
   [`internal/lsp/protocol.go`](../internal/lsp/protocol.go).
   New fields on `workspaceEdit`:
   `documentChanges` and `changeAnnotations`.
   New types: `annotatedTextEdit`,
   `textDocumentEdit`, `changeAnnotation`. Keep
   `changes` for the legacy path.
4. [x] Switch the `Changes` JSON tag on `workspaceEdit`
   to `changes,omitempty` so the annotated path
   emits only `documentChanges` and
   `changeAnnotations`. Refactor `fullFileEdit`
   (and any other edit builder) to pick one shape
   per call. Mode is
   `preview ∧ clientSupportsAnnotations`.
5. [x] Per-rule annotation IDs in `computeCodeActions`,
   keyed off the same `d.Data.RuleName` the
   existing quickfix path uses:
   `mdsmith-fix-<rule-name>` for quick fixes,
   `mdsmith-fix-all` for `source.fixAll.mdsmith`.
   Label reuses the existing `quickFixTitle(rule)`
   verbatim.
6. [x] Add the `mdsmith.previewFix` setting to
   [`editors/vscode/package.json`](../editors/vscode/package.json)
   contributions with description and default
   `false`.
7. [x] Document the setting in
   [`docs/guides/editors/vscode.md`](../docs/guides/editors/vscode.md)
   (Settings table + a short "Preview before
   applying" subsection).
8. [x] Document the LSP behavior in
   [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
   — capability requirement and the wire shape.
9. [x] Tests:

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

- [x] `mdsmith.previewFix=false` (the default)
      leaves the LSP `WorkspaceEdit` byte-identical
      to today's output for both `quickfix` and
      `source.fixAll.mdsmith`.
- [x] `mdsmith.previewFix=true` + a client that
      advertises `changeAnnotationSupport`
      produces an edit using `documentChanges`
      with `changeAnnotations[*].needsConfirmation
      = true`.
- [x] Without either `documentChanges` or
      `changeAnnotationSupport` on the client, the
      server falls back to the legacy `changes`
      shape and logs the fallback once per
      session.
- [x] `source.fixAll.mdsmith` carries the
      `mdsmith-fix-all` annotation. (Revised: quick
      fixes no longer carry a per-rule annotation —
      they apply immediately. See the Revision note.)
- [x] [`docs/guides/editors/vscode.md`](../docs/guides/editors/vscode.md)
      documents the setting and the
      `previewFix` × `fixOnSave` interaction.
- [x] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      documents the new wire shape and the
      capability gate.
- [x] All tests pass: `go test ./...` and
      `bun test` in `editors/vscode`.
- [x] `go tool golangci-lint run` reports no
      issues.
