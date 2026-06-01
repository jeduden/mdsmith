---
id: 212
title: "`mdsmith extract` projects paragraph inline spans as data"
status: "🔳"
summary: >-
  Add a content-entry projection mode that emits a paragraph's
  inline spans (text, emphasis, strong, code, link) as a
  structured list. Consumers stop walking AST or matching
  Markdown markup by hand; they read the typed segments from
  the extract JSON.
model: opus
depends-on: [210]
---
# `mdsmith extract` projects paragraph inline spans as data

## Goal

A paragraph's plain `text` projection works for prose where
rendering is up to the consumer. It fails when the consumer
needs the structure inside the paragraph. Which span is
emphasised. Which token is a link. Which fragment is code.

Today `paragraph → text` drops every inline mark. Downstream
consumers walk the AST themselves. `internal/release/messaging.go`
parses the messaging headline's `*…*` span by re-running the
goldmark parser inside the release tool. Others reach for
regex on raw markup.

After this plan, a content entry can declare an `inline`
projection that emits a typed list of spans:

```yaml
sections:
  - heading: { regex: '^Headline$' }
    content:
      - { kind: paragraph, projection: inline, required: true }
```

…and extract emits the recursive shape. Container spans
carry `children`. Leaf spans carry `value`. For the headline
`Mark*down*, smithed.`:

```json
"headline": {
  "inline": [
    { "span": "text", "value": "Mark" },
    { "span": "emphasis", "level": 1, "children": [
      { "span": "text", "value": "down" }
    ]},
    { "span": "text", "value": ", smithed." }
  ]
}
```

Nesting is supported uniformly: a `**`mdsmith fix`**` strong
containing a code span emits

```json
{ "span": "strong", "level": 2, "children": [
    { "span": "code", "value": "mdsmith fix" }
] }
```

Consumers walk one shape — no flat-vs-recursive mode switch,
no fail mode for content that happens to nest.

Then `internal/release/messaging.go` reads `headline.inline`
and looks for the one `emphasis` span. No Markdown parsing on
the release side. Just a typed walk over data.

## Tasks

1. [x] **Content-projection field on schema.** Add a `projection:`
   key to schema content entries. Allowed values: `text` (the
   current default), `code` (for code blocks, already
   implicit), and `inline` (new). Validate at schema-load time;
   reject `projection: inline` on non-paragraph kinds.
2. [x] **AST → typed-span walker.** In
   [`internal/extract`](../internal/extract), implement the
   inline-span walker. Container spans (emphasis, strong,
   link) carry `children`; leaf spans (text, code, autolink)
   carry `value`. The mapping from goldmark AST to span object:

   | AST node               | Emitted span                                  |
   | ---------------------- | --------------------------------------------- |
   | `ast.Text`             | `{span: text, value}`                         |
   | `ast.CodeSpan`         | `{span: code, value}`                         |
   | `ast.AutoLink`         | `{span: autolink, value, url}`                |
   | `ast.Emphasis` Level 1 | `{span: emphasis, level: 1, children: [...]}` |
   | `ast.Emphasis` Level 2 | `{span: strong, level: 2, children: [...]}`   |
   | `ast.Link`             | `{span: link, url, title?, children: [...]}`  |

   Container spans recurse through the same walker, so nesting
   composes naturally. Anything not in this table (raw HTML,
   images, custom inline) is a hard error from extract.
3. [x] **YAML / msgpack passthrough.** The same projection mode
   works for `--format yaml` and `--format msgpack`. The
   in-memory tree is one shape; only the serializer changes.
4. **Default-key collision.** A scope declaring both
   `{kind: paragraph, projection: text}` and
   `{kind: paragraph, projection: inline}` would emit two
   sibling keys; declare and document the default keys
   (`text` and `inline`) so a schema author can resolve a
   collision via `bind:`.
5. **Adopt in messaging.** Switch
   [`docs/brand/messaging.md`](../docs/brand/messaging.md)'s
   `## Headline` from a code block to a paragraph with
   `projection: inline`. Drop the
   [`parseHeadlineEmphasis`][parser] helper. Drop the
   import of `pkg/markdown` from `internal/release/`. The
   release tool replaces the AST walker with a typed walk:
   find the first `emphasis` span at the top level, flatten
   its `children` to text (rejecting non-text children), pre
   / em / post fall out from the sibling positions.
   `mdsmith-release sync-messaging --check` stays clean.
6. **Documentation.** Add an "Inline-span projection"
   subsection to the
   [extract reference](../docs/reference/cli/extract.md)
   showing the mapping table and a nesting example, and a
   worked example to the
   [extract-markdown-as-data guide](../docs/guides/extract-markdown-as-data.md).

[parser]: ../internal/release/messaging.go

## Acceptance Criteria

- [ ] A paragraph content entry with `projection: inline`
  emits a `{inline: [...]}` object where each element is a
  typed span (text / emphasis / strong / code / link /
  autolink). Container spans carry `children`; leaf spans
  carry `value`.
- [ ] Nested inline (`**`code`**`, `[**bold**](url)`, etc.)
  round-trips through the projection without error; the
  consumer walks one uniform shape.
- [ ] `internal/release/` no longer imports `pkg/markdown` or
  parses Markdown itself. The headline parsing helper is
  deleted; the release tool reads `headline.inline` directly.
- [ ] `mdsmith extract` rejects an unsupported inline node
  (raw HTML, image, custom) when the schema asks for `inline`.
- [ ] The mapping table is documented in the extract
  reference; the worked example in the guide shows both
  schema and JSON output (including one nested case).
- [ ] `mdsmith check .` clean; `mdsmith-release sync-messaging
  --check` reports no drift on the messaging surfaces.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no issues.
