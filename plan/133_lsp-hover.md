---
id: 133
title: LSP hover for rule and directive docs
status: "✅"
model: sonnet
summary: >-
  Add `textDocument/hover` to `mdsmith lsp` so editors
  and Claude Code see rule help on hover over a
  diagnostic squiggle and directive docs on hover
  inside a `<?…?>` block. Rule docs reuse the body
  `mdsmith help rule <id>` prints; directive docs
  come from the existing files under
  `docs/guides/directives/`.
---
# LSP hover for rule and directive docs

## Goal

Surface rule and directive documentation inline in
the editor without leaving the buffer. Hovering
over an `MDS001` squiggle should show the
line-length help body; hovering inside a
`<?catalog?>` block should show the catalog
directive docs. Same content the CLI prints, no
duplication.

## Background

Plan 121 shipped diagnostics and code actions.
Plan 131 shipped navigation. Hover is the only
surface in Claude Code's [code intelligence][cc-ci]
list still missing from `mdsmith lsp` ("get type
info on hover").

[cc-ci]: https://code.claude.com/docs/en/discover-plugins#code-intelligence

Plan 122 originally bundled hover with a set of VS
Code palette commands. The two ship independently:
hover is server-side and benefits every LSP client
(Claude Code, Neovim, Helix, JetBrains); the
palette is VS Code-only chrome. This plan extracts
the hover work so it can land without waiting on
the palette decisions.

## Non-Goals

- Hover content for non-mdsmith concepts (front-
  matter schemas, Markdown spec quirks). Scope is
  rule docs and directive docs only.
- A new `mdsmith help directives` CLI topic. The
  directive content is loaded directly by the LSP
  server from the existing files under
  [docs/guides/directives/](../docs/guides/directives/).
- Hover on link targets, headings, or kind values.
  Definition / implementation already cover those
  with a click-through; hover here is reserved for
  documentation, not navigation preview.

## Design

### Capability

Advertise `hoverProvider = true` in the server's
`initialize` response.

### Resolution order

A `textDocument/hover` request arrives with a
position. The server resolves in two passes:

**Diagnostic-first.** If the position falls inside
any active diagnostic range for the document, the
server returns a `MarkupContent` with kind
`markdown`. The body holds the diagnostic message
on one line, a blank line, then the rule's docs.
Rule docs load via
[`internal/rules.LookupRule`](../internal/rules/ruledocs.go)
— the same lookup `mdsmith help rule <id|name>`
uses.

**Directive fallback.** If no diagnostic covers the
cursor, the server checks for a `<?directive ...?>`
block at that position (via
[`lint/pi_parser.go`](../internal/pi/pi_parser.go)).
If one matches, the server returns the directive
docs from [docs/guides/directives/](../docs/guides/directives/).

If neither matches, the server returns `null` (no
hover).

### Range hint

Each hover response sets `range` to the matched
span — the diagnostic range, or the directive
block range. Clients render the hover anchored to
that span.

### Markdown safety

VS Code, Claude Code, and most LSP clients render
hover content as Markdown and strip inline HTML.
Rule and directive docs in this repo today are
clean of HTML outside code blocks, but a future
contributor could add a `<span>` or `<details>`
that would silently disappear from the hover.

This plan adds a preventive guard: enable
`no-inline-html` (MDS041) on the existing
`rule-readme` kind in [.mdsmith.yml](../.mdsmith.yml).
The rule is opt-in by default. The same kind
already targets [`internal/rules/MDS*/README.md`](../internal/rules)
and pins `required-structure.schema`. Adding the
opt-in setting:

```yaml
kinds:
  rule-readme:
    rules:
      required-structure:
        schema: internal/rules/proto.md
      no-inline-html:
        allow: [kbd]
```

A spot audit of the existing rule README files
found no raw HTML outside code blocks; this is
purely preventive.

Editing `.mdsmith.yml` requires explicit user
consent per [CLAUDE.md](../CLAUDE.md). The task
that lands the kind change surfaces the diff
first.

### Backwards compatibility

`hoverProvider` is additive. Clients that ignore
the capability see the same server. The
`no-inline-html` opt-in only fires on the
`rule-readme` kind; no other Markdown is affected.

## Tasks

1. ✅ Wire hover's rule lookup through
   [`internal/rules`](../internal/rules) APIs
   (`LookupRule(query) (string, error)` and
   `ListRules()`). Cover known rules, unknown
   rules, and rules whose docs have no
   rule-specific body (fall back to a generic
   "see `mdsmith help rule <id>`" line).
2. ✅ Add `textDocument/hover` to the LSP server in
   [`internal/lsp/server.go`](../internal/lsp/server.go).
   Implement the diagnostic-first then directive-
   fallback resolution. Return `null` when neither
   matches.
3. ✅ Add a directive-doc loader that reads from
   [docs/guides/directives/](../docs/guides/directives/)
   keyed by directive name. Cache the parsed
   contents on first read.
4. ✅ Add an integration test in
   [`cmd/mdsmith`](../cmd/mdsmith) that drives an
   `initialize` → `didOpen` → `hover` round trip
   for both the diagnostic and directive cases,
   and the no-match case.
5. ✅ Add `hoverProvider` to the capability table in
   [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
   and document the resolution order.
6. ✅ Propose enabling `no-inline-html` on the
   `rule-readme` kind in [.mdsmith.yml](../.mdsmith.yml).
   Surface the diff to the user before applying.
   Add a regression test fixture: a rule README
   with a raw `<span>` outside a code block fails
   `mdsmith check` with `MDS041`.

## Acceptance Criteria

- [x] `hoverProvider` appears in the
      `initialize` capabilities response.
- [x] Hovering over an `MDS006` diagnostic
      returns a `MarkupContent` whose body
      contains the rule help text.
- [x] Hovering inside a `<?catalog?>` directive
      returns the catalog directive docs even when
      no diagnostic is present at the cursor.
- [x] Hovering on plain prose (no diagnostic, no
      directive) returns `null`.
- [x] After the `rule-readme` kind change lands,
      a fixture rule README containing a raw
      `<span>` outside a code block fails
      `mdsmith check` with `MDS041`.
- [x] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      lists `hoverProvider` in the capability
      table.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
