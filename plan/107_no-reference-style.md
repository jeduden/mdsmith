---
id: 107
title: No reference-style links rule
status: "✅"
summary: >-
  New rule MDS043 that forbids reference-style links
  and footnotes. These constructs require global
  definition resolution, moving Markdown from a
  context-free to a context-sensitive grammar — the
  "Exhibit D" complaint in the bgslabs.org rant.
model: opus
---
# No reference-style links rule

## Goal

Let users forbid the link forms whose meaning depends
on declarations elsewhere in the document. Inline
links (`[text](url)`) are fully local. Reference
links (`[text][id]` plus `[id]: url`) and footnotes
(`[^n]` plus `[^n]: ...`) require a global pass over
the file to resolve. Forbidding them keeps every link
diff readable in isolation and removes one of the two
cases that pushes Markdown grammar above context-free.

## Background

### What goldmark exposes

- Inline links → `*ast.Link` with non-nil `Destination`
  on the node itself.
- Reference links → also `*ast.Link`, but resolved
  via `parser.Reference` that goldmark stores in the
  document context. The original source uses the
  `[text][id]` shape.
- Reference definitions → not an AST node by default;
  they are consumed during parsing and never appear
  in the tree.
- Footnotes → `*ast.Footnote` and
  `*ast.FootnoteReference`, available only when the
  footnote extension is enabled.

The check must inspect the *source*, not just the
AST. Goldmark resolves inline links and reference
links to the same `*ast.Link` node. The original
shape — `[text](url)` vs `[text][id]` vs `[text][]`
— is recoverable only from the source bytes.

### Why a separate rule from MDS034

MDS034 (markdown-flavor) flags footnotes as a flavor
extension. MDS043 forbids them even on flavors that
support them, because the *grammar* concern is
independent of the renderer concern. The two rules
can be enabled together; MDS034 fires when the flavor
is `commonmark` and the file uses footnotes, MDS043
fires when the policy forbids footnotes regardless
of flavor.

## Design

### Configuration

```yaml
rules:
  no-reference-style:
    allow-footnotes: false   # opt back in if needed
```

Category: `link`. Disabled by default (opt-in).

When `allow-footnotes: true`, footnote references
are accepted under two constraints. The reference
must use the `[^slug]` shape with a meaningful slug.
The definition must sit immediately after the
referencing paragraph. Numeric `[^1]` is rejected
because the number carries no anchor.

### Flavor activation

[Plan 112](112_flavor-profiles.md) ships profiles
that auto-enable this rule:

- `profile: portable` activates with
  `allow-footnotes: false`.
- `profile: github` does not activate
  this rule.
- `profile: plain` activates with
  `allow-footnotes: false`.

User overrides on top of the profile still win via
deep-merge.

### Detection

Walk `*ast.Link` nodes. For each link, read the
source bytes between the closing `]` and the next
non-whitespace character:

- `(` → inline link, accept.
- `[` → reference or collapsed reference, flag as
  `reference-style link`.
- nothing → shortcut reference (`[text]` alone),
  flag as `reference-style link`.

Walk `*ast.FootnoteReference`. When `allow-footnotes`
is false, flag every occurrence. When true, validate
the slug format and the definition placement.

Reference *definitions* (`[id]: url`) emit a
diagnostic of their own when no inline-style link
in the file uses the `id`, since they are dead code.
When the file has reference-style links, the link
diagnostics already cover the issue and the
definition is left alone.

### Auto-fix

Reference-style → inline: substitute the resolved
URL into the link, drop the definition. Possible
when goldmark already resolved the reference.

Footnote → inline: not auto-fixed. The footnote text
is meant to be visually separated; turning it into
`(...)` parentheticals changes the document.

### Error messages

```text
reference-style link; use inline form [text](url)
footnote reference; footnotes are not allowed
footnote slug is numeric; use a meaningful slug
unused reference definition: [{id}]
```

## Tasks

- [x] Scaffold `internal/rules/noreferencestyle/`.
- [x] Implement source-aware link form detection.
- [x] Implement footnote checks gated on
      `allow-footnotes`.
- [x] Implement unused-definition detection.
- [x] Implement inline-rewrite auto-fix for
      reference-style links only.
- [x] Register as MDS043 in category `link`.
- [x] Add fixture tests covering inline, full
      reference, collapsed reference, shortcut
      reference, footnote (with and without
      `allow-footnotes`), unused definition, and
      numeric footnote slug.
- [x] Add rule README.

## Acceptance Criteria

- [x] `[text](url)` emits no diagnostic.
- [x] `[text][id]` plus `[id]: url` emits one
      diagnostic per link occurrence and fixes to
      `[text](url)`.
- [x] `[text][]` collapsed reference emits one
      diagnostic.
- [x] `[text]` shortcut reference (with matching
      definition) emits one diagnostic.
- [x] `[^1]` with `allow-footnotes: false` emits one
      diagnostic.
- [x] `[^1]` with `allow-footnotes: true` emits one
      diagnostic for numeric slug.
- [x] `[^slug]` with `allow-footnotes: true` and
      definition right after the paragraph emits no
      diagnostic.
- [x] `[id]: url` with no link referencing it emits
      one diagnostic.
- [x] Rule is disabled by default.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
