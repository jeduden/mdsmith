---
id: 122
title: Hover and CodeLens for help and kinds in VS Code
status: "🔲"
model: sonnet
summary: >-
  Add `textDocument/hover` and `textDocument/codeLens`
  to the LSP server so rule help text shows on hover
  over a diagnostic and a per-file CodeLens summarizes
  the resolved kinds with a click-through to the full
  merge chain.
---
# Hover and CodeLens for help and kinds in VS Code

## Goal

Surface `mdsmith help` and `mdsmith kinds` content
inline in the editor without leaving the buffer.
Hovering on a diagnostic shows the rule's help
body. A CodeLens at the top of each Markdown file
names the resolved kinds. Clicking opens the full
per-leaf merge chain.

## Background

Plan 121 wires the LSP transport and publishes
diagnostics. With diagnostics on screen, the next
question a user asks is "what does this rule mean
and why does it apply to this file?". Today the
answer lives in two CLI subcommands the editor
cannot reach: `mdsmith help <rule>` and
`mdsmith kinds why <file> <rule>`.

LSP has dedicated capabilities for both surfaces.
`textDocument/hover` returns a Markdown payload
keyed by document position. `textDocument/codeLens`
returns clickable annotations attached to ranges in
the document. Both reuse the engine the server
already loaded — no new shell-outs.

## Design

### Hover

Capability: `hoverProvider = true`.

A `textDocument/hover` request arrives with a
position. When the position falls inside a
diagnostic range, the server returns Markdown:

- The diagnostic message (one line).
- The rule's `help` body, loaded the same way
  `mdsmith help <rule>` loads it.
- A trailer link `Why does this apply?` whose URI is
  `command:mdsmith.kinds.why?<file>&<rule>`. The
  client extension registers the command and opens
  a virtual document with the
  `mdsmith kinds why --json` output rendered as
  Markdown.

When no diagnostic covers the position, the server
checks for a `<?directive?>` block. If one is
under the cursor, the hover returns that
directive's docs. The source is the same body
`mdsmith help directives` prints.

### CodeLens

Capability: `codeLensProvider = {resolveProvider:
true}`.

One CodeLens per Markdown file, anchored to line 1.
Title format: `Kinds: <name>, <name> • <N> rules`.
The kind list and rule count come from
`internal/kindsout` (the same source
`mdsmith kinds resolve --json` uses).

A click runs the `mdsmith.kinds.resolve` extension
command. The handler opens a virtual document with
the resolved rule bodies.

Resolution is lazy. The first response returns the
lens without a `command` field. The client then
sends `codeLens/resolve` once the lens scrolls
into view. The server fills in the title and
command at that point.

### Reuse

The server packages already load defaults, kinds,
and overrides during `initialize`. The hover and
CodeLens handlers reuse:

- [`internal/rules`](../internal/rules) for the
  per-rule help body lookup.
- [`internal/kindsout`](../internal/kindsout) for
  the resolve / why output.
- The cached merge results from plan 121's
  document store.

No new disk reads on the hover path.

## Tasks

1. Expose a `Help(ruleID) (string, bool)` entry
   point on
   [`internal/rules`](../internal/rules) returning
   the same Markdown the CLI prints. Add unit tests
   covering known rules, unknown rules, and rules
   with no help body.
2. Add `textDocument/hover` to the LSP server.
   Match the position against the active
   diagnostic ranges; fall through to the directive
   index when no diagnostic covers the cursor.
3. Add `textDocument/codeLens` and
   `codeLens/resolve` to the LSP server. Anchor
   one lens at line 1 with the resolved-kinds
   summary.
4. Register `mdsmith.kinds.why` and
   `mdsmith.kinds.resolve` commands in the VS Code
   extension. Each opens a virtual document with
   the JSON output rendered as Markdown.
5. Add an integration test under
   [`cmd/mdsmith`](../cmd/mdsmith) that drives the
   LSP server through `initialize` →
   `textDocument/hover` and asserts the response
   contains the rule's help body.
6. Update the VS Code guide
   `docs/guides/editors/vscode.md` (created in
   plan 121) with screenshots and a description of
   the hover and CodeLens flows.

## Acceptance Criteria

- [ ] Hovering over a `MDS001` squiggle in VS Code
      shows the line-length help body inline.
- [ ] Hovering inside a `<?catalog?>` directive
      shows the catalog directive docs even when no
      diagnostic is present.
- [ ] Each Markdown file shows one CodeLens at line
      1 listing the resolved kinds and rule count;
      the count matches `mdsmith kinds resolve
      --json`.
- [ ] Clicking the CodeLens opens a virtual
      document with the merged rule bodies; closing
      it leaves no temp file behind.
- [ ] The "Why does this apply?" link in the hover
      opens the `kinds why` view for the diagnostic
      under the cursor.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.

## Open Questions

- **Markdown rendering inside hovers.** VS Code
  renders LSP hover Markdown but strips inline
  HTML. Confirm the rule-help bodies do not rely
  on raw HTML; if any do, route them through a
  sanitiser before sending.
- **CodeLens cost.** A `codeLens` request fires on
  every file open and on every edit when the
  request is not cached. Measure the overhead on a
  100-file workspace before committing to lazy
  resolution as the only optimisation.

## ...

<?allow-empty-section?>
