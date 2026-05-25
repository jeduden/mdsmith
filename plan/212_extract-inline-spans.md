---
id: 212
title: "`mdsmith extract` projects paragraph inline spans as data"
status: "🔲"
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

…and extract emits:

```json
"headline": {
  "inline": [
    { "kind": "text",     "value": "Mark" },
    { "kind": "emphasis", "value": "down", "level": 1 },
    { "kind": "text",     "value": ", smithed." }
  ]
}
```

Then `internal/release/messaging.go` reads `headline.inline`
and looks for the one `emphasis` span. No Markdown parsing on
the release side. Just a typed walk over data.

## Tasks

1. **Content-projection field on schema.** Add a `projection:`
   key to schema content entries. Allowed values: `text` (the
   current default), `code` (for code blocks, already
   implicit), and `inline` (new). Validate at schema-load time;
   reject `projection: inline` on non-paragraph kinds.
2. **AST → typed-span walker.** In
   [`internal/extract`](../internal/extract), implement the
   inline-span walker. The mapping from goldmark AST to span
   object:

   | AST node               | Emitted span                        |
   | ---------------------- | ----------------------------------- |
   | `ast.Text`             | `{kind: text, value}`               |
   | `ast.Emphasis` Level 1 | `{kind: emphasis, value, level: 1}` |
   | `ast.Emphasis` Level 2 | `{kind: strong, value, level: 2}`   |
   | `ast.CodeSpan`         | `{kind: code, value}`               |
   | `ast.Link`             | `{kind: link, value, url, title?}`  |
   | `ast.AutoLink`         | `{kind: autolink, value, url}`      |

   Anything else (raw HTML, images, custom inline) is a hard
   error. The consuming schema declared it wanted typed-only
   content.
3. **YAML / msgpack passthrough.** The same projection mode
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
   import of `pkg/markdown` from `internal/release/`.
   `mdsmith-release sync-messaging --check` stays clean.
6. **Documentation.** Add an "Inline-span projection"
   subsection to the
   [extract reference](../docs/reference/cli/extract.md)
   showing the mapping table, and a worked example to the
   [extract-markdown-as-data guide](../docs/guides/extract-markdown-as-data.md).

[parser]: ../internal/release/messaging.go

## Acceptance Criteria

- [ ] A paragraph content entry with `projection: inline`
  emits a `{inline: [...]}` object where each element is a
  typed span (text / emphasis / strong / code / link /
  autolink).
- [ ] `internal/release/` no longer imports `pkg/markdown` or
  parses Markdown itself. The headline parsing helper is
  deleted; the release tool reads `headline.inline` directly.
- [ ] `mdsmith extract` rejects an unsupported inline node
  (raw HTML, image, custom) when the schema asks for `inline`.
- [ ] The mapping table is documented in the extract
  reference; the worked example in the guide shows both
  schema and JSON output.
- [ ] `mdsmith check .` clean; `mdsmith-release sync-messaging
  --check` reports no drift on the messaging surfaces.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no issues.
