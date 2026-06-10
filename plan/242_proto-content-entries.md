---
id: 242
title: "proto.md schemas declare content entries via `<?content?>`"
status: "🔲"
summary: >-
  A `<?content?>` directive row in a proto.md section body
  declares the same content entry the inline `content:` list
  declares, so file-based kinds validate and extract body
  content instead of projecting empty section objects.
model: opus
depends-on: []
---
# proto.md schemas declare content entries via `<?content?>`

## Goal

A kind backed by a `proto.md` file schema validates headings
only. The proto surface cannot express the inline schema's
`content:` entries. `mdsmith extract` therefore projects
every section of a proto-based kind as an empty object:

```json
"tagline": {}
```

The inline form of the same section declares
`content: [{ kind: paragraph }]` and projects
`"tagline": { "text": "…" }`. This asymmetry forces any
extraction-centric kind onto the inline form. The inline form
cannot root at H1, so such a kind can never combine body
extraction with the `# {title}` heading sync
(see the
[extract guide](../docs/guides/extract-markdown-as-data.md)).

After this plan, a `<?content?>` directive row in a proto
section body declares one content entry. The directive body
carries the same keys as an inline entry:

```markdown
# {title}

## Tagline

<?content
kind: paragraph
projection: inline
?>
```

Two directives in one section declare two ordered entries
(`text` / `text-2` keys behave as in the inline form). The
precedent is `<?require filename: "…"?>`, which already lives
in proto bodies as a directive row.

## Design notes

- The schema-package proto parser
  ([`internal/schema/parse_file.go`](../internal/schema/parse_file.go))
  parses the directive into the same `Content` slice the
  inline parser fills. Validation reuses the inline rules
  (`projection: inline` only on `kind: paragraph`, …).
- MDS020's legacy file-schema parser must skip `<?content?>`
  rows so they are not treated as body-sync template text.
  Full content validation for proto-based kinds rides the
  plan-156 parser that `mdsmith extract` already uses.
- `bind:` and `required:` keys pass through unchanged.

## Tasks

1. Parse `<?content?>` rows in the schema-package proto
   parser into content entries; reject unknown keys and
   invalid kind/projection combinations at parse time with
   the same diagnostics as the inline form.
2. Teach the legacy MDS020 proto parser to skip
   `<?content?>` rows (no body-sync interpretation, no
   diagnostic).
3. Wire extraction: a proto-based kind with `<?content?>`
   entries projects body content identically to the
   equivalent inline schema. Differential test: one schema
   expressed both ways, byte-identical extract output
   (modulo root level).
4. Document the directive in the
   [section-schema reference](../docs/reference/section-schema.md)
   proto.md section and in the
   [schemas guide](../docs/guides/schemas.md); update the
   inline-vs-proto capability table.
5. Revisit the
   [extract guide](../docs/guides/extract-markdown-as-data.md)
   "Frontmatter `title` and the H1" section: the
   content-entries limit no longer applies; rewrite the
   trade-off paragraph.

## Acceptance Criteria

- [ ] A proto.md section body may contain `<?content?>`
  directive rows; each declares one content entry with the
  inline keys (`kind`, `projection`, `required`, `bind`).
- [ ] `mdsmith extract` on a proto-based kind with declared
  content projects the same tree as the equivalent inline
  schema.
- [ ] MDS020 (legacy path) neither flags nor body-syncs a
  `<?content?>` row.
- [ ] Invalid directives (unknown kind, `projection: inline`
  on a code-block, `bind: ""` on a content entry) fail at
  config load with the inline form's diagnostics.
- [ ] Docs updated: section-schema reference, schemas guide
  capability table, extract guide trade-off paragraph.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues

## Out of scope

- Expressing `repeat:` / `sequential:` in proto.md (existing
  limitation, unchanged).
- The MDS020 cutover from the legacy parser to the
  schema-package parser (separate effort; this plan only
  makes the legacy parser skip the new rows).
