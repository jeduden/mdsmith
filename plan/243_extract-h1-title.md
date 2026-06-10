---
id: 243
title: "`mdsmith extract` projects the document H1 as `title`"
status: "🔲"
summary: >-
  When the composed schema roots at H2, extract emits the
  document H1's rendered plain text under a reserved top-level
  `title` key beside `frontmatter`, so consumers read the title
  without duplicating it into a frontmatter field.
model: sonnet
depends-on: []
---
# `mdsmith extract` projects the document H1 as `title`

## Goal

The H1 is the document's title, but `mdsmith extract` cannot
project it. Inline schemas root at H2, so the H1 is outside
every scope. The only workaround is a proto.md `# {title}`
row, and that requires a frontmatter `title` field that
duplicates the H1. An unresolved `{title}` row degrades to a
wildcard, and extract skips wildcard scopes. The
[extract guide](../docs/guides/extract-markdown-as-data.md)
documents this as a current limit.

After this plan, when the composed schema roots at H2, the
projector emits the document H1's rendered plain text under a
reserved top-level `title` key:

```json
{
  "frontmatter": {},
  "title": "Product copy",
  "tagline": { "text": "…" }
}
```

Files stop needing a frontmatter `title:` whose only consumer
wanted the document title as data. `<?include extract:
title?>` splices it.

## Design notes

- **Reserved key, no opt-in.** The key is `title`, emitted
  beside `frontmatter` whenever the schema roots at H2 and
  the document has an H1. No H1 → key omitted (consistent
  with optional sections: omitted, not null).
- **Rendered plain text.** Same renderer the heading matcher
  uses: emphasis and code-span markers stripped, link text
  kept, attribute lists and trailing ATX `#`s dropped.
- **Collisions.** A sibling scope that resolves to the key
  `title` (slug or `bind:`) is a collision, reported through
  the existing sibling-collision machinery; the schema author
  resolves it with `bind:`.
- **H1-rooted schemas unchanged.** When the composed schema
  roots at H1 (proto.md), the H1 is already a scope; no
  reserved key is added.
- **First H1 wins** when a document carries more than one.

## Tasks

1. Emit the reserved `title` key in
   [`internal/extract`](../internal/extract) when the
   composed schema's root level is 2 and the document has an
   H1; omit it otherwise. Unit-test: present, absent,
   emphasis-bearing, and multi-H1 documents.
2. Route the key through the existing collision check; test a
   scope bound to `title` reports a collision naming both
   sources.
3. Verify `<?include extract: title?>` splices the H1 text
   (the include path walks the same projection).
4. Update the
   [extract reference](../docs/reference/cli/extract.md)
   default-projection section and the
   [extract guide](../docs/guides/extract-markdown-as-data.md):
   the "cannot project the H1 without a frontmatter field"
   limit is gone; rewrite the closing paragraph of the
   "Frontmatter `title` and the H1" section and its verified
   outputs.

## Acceptance Criteria

- [ ] `mdsmith extract` on an H2-rooted kind emits
  `"title": "<H1 plain text>"` at the top level; the key is
  omitted when the document has no H1.
- [ ] A sibling projection that resolves to `title` is
  reported as a collision before any data is emitted.
- [ ] H1-rooted (proto.md) schemas produce unchanged output.
- [ ] `<?include extract: title?>` splices the H1 text.
- [ ] Reference and guide updated with verified outputs.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues

## Out of scope

- Inline-span projection of the H1 (rides the block grammar,
  [plan 246](246_block-projection-full-extract.md)).
- Enforcing H1 ↔ frontmatter consistency (already covered by
  proto.md heading sync).
