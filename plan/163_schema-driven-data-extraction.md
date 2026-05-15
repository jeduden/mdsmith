---
id: 163
title: Schema-driven data extraction (mdsmith extract)
status: "🔲"
model: opus
depends-on: [149, 156]
summary: >-
  Derive a default data tree from the hierarchical
  schema and add an `extract` subcommand that emits a
  kind-conformant file as JSON/YAML/Lua/msgpack.
---
# Schema-driven data extraction (mdsmith extract)

## Goal

Let a kind's schema double as an extraction contract.
Once `mdsmith check` confirms a file conforms, `mdsmith
extract <kind> --format json|yaml|lua|msgpack <file>`
emits a data tree. Its shape is derived from the schema
hierarchy itself — no annotations required.

## Why a default binding layer first

The schema is already a hierarchy: front matter, then a
tree of scopes (sections), each with child scopes and
content entries. That hierarchy *is* the data shape. So
the first deliverable is a **default binding layer** that
projects the schema tree into a data tree directly,
mirroring its nesting. No new schema concept is needed
for the common case.

Custom shaping is *not* in this plan. It is a separate
follow-up — [plan 164](164_custom-binding-overrides.md) —
and we keep it cheap by design: every key flows through
one `keyFor(node)` seam (task 3), so the override plan is
a focused change there plus parsing `bind:`. Until then,
renaming or restructuring is the job of a downstream tool
(`jq`, `yq`, a Lua script) over the standard-format
output.

## Default projection rules

The projection walks the composed schema in lockstep with
the validated match and mirrors the hierarchy:

- **Front matter** → top-level `frontmatter` object,
  passed through from the existing decode.
- **Literal-heading scope** (`## Goal`) → object keyed by
  the slugified heading (`goal`), reusing the existing
  anchor slugifier. Its value holds child scopes and
  content, recursively.
- **Repeating scope** (`## {id}`, `repeats: true`) → an
  array keyed by the slug of the heading's literal stem
  (or, if none, the placeholder name). Each element is an
  object whose fields are the captured placeholders plus
  the element's own child scopes and content.
- **`code-block`** → string under `code` (raw body);
  multiple blocks get `code`, `code-2`, …
- **`list`** → array of item strings under `items`.
- **`table` with `columns`** → array of row objects keyed
  by column header, under `rows`.
- **`paragraph`** → its text under `text`.

Sibling key collisions (two `## Goal` headings, or a
content default that shadows a child scope slug) are a
schema error reported at extract time, pointing at the
schema source. Empty/optional sections that did not match
are omitted rather than emitted as null.

## Sequencing

The schema engine is mid-rework. This plan lands after,
and consumes the outputs of, that work — not the legacy
single-source model.

- **[Plan 156 — kind-schema
  composition](156_kind-schema-composition.md) / PR
  #288.** (Disambiguation: two plan files share id 156;
  this dependency is the composition one, not
  `156_schema-entry-unification.md`.) A file can resolve
  to multiple kinds whose schemas compose via
  `schema.Compose()`. The extractor consumes the composed
  `Schema`. Default keys derive from heading text, so
  identical headings from two kinds merge to the same key
  with no conflict; only genuinely divergent shapes
  surface as a collision.
- **Plan 149 (section-content schema).** Content
  projection rides on the `ContentEntry` model from the
  content-schema work. This plan adds no content matcher
  of its own and is blocked until that model is stable.
- **Plan 147 / PR #284 (actionable schema diagnostics).**
  If landed, collision and conformance failures reuse the
  `SchemaDiagnostic` formatter.

Extraction is gated on a successful schema match. A
non-conformant file makes `extract` report the same
diagnostics as `check` and exit non-zero. It never emits
partial data.

## Tasks

1. **Expose the match tree.** Refactor `schema.Validate`
   (and the content matcher) to also return a new
   `*schema.MatchTree` in `internal/schema`: for each
   `Scope` / `ContentEntry`, the matched AST nodes, their
   source lines, and captured `{field}` values. `Validate`
   keeps its diagnostic return; the tree is an added
   result so MDS020 is unaffected. Unit-test the tree on
   the existing schema fixtures.
2. **Extractor skeleton (red/green).** Add
   `internal/extract` with `Extract(f *lint.File, sch
   *schema.Schema, m *schema.MatchTree) (any,
   []lint.Diagnostic)`. `sch` is the composed schema; `m`
   is the tree from task 1 — no re-matching.
3. **Default scope projection.** Walk the scope tree and
   build the nested object/array structure per the rules
   above. Route every key through one `keyFor(node)`
   function — the single seam a future custom-binding plan
   overrides. Reuse the existing anchor slugifier. Unit-
   test literal, nested, and optional-omitted scopes.
4. **Repeating scopes and placeholders.** Project
   `repeats: true` scopes as arrays; record each captured
   `{field}` into the element object, reusing
   [fieldinterp](../internal/fieldinterp/fieldinterp.go).
5. **Default content projection.** Project `code-block`,
   `list`, `table`, and `paragraph` entries (plan 149)
   with their default keys. Detect sibling key collisions
   and emit a schema diagnostic.
6. **Composition behavior.** Add `compose_test.go` /
   extractor tests proving a file under two kinds yields a
   merged tree, and that a real shape divergence is
   reported as a collision, not silently dropped.
7. **Format encoders.** Add `internal/extract/encode`
   with json (stdlib), yaml (existing dep), msgpack, and
   lua (table literal) encoders behind a `Format` enum.
8. **`extract` subcommand.** Register `extract` in
   [main.go](../cmd/mdsmith/main.go); signature `mdsmith
   extract <kind> --format <fmt> <file>`. Reuse the
   config-load and kind-resolution helpers from
   [kinds.go](../cmd/mdsmith/kinds.go). Validate that
   `<kind>` is one of the file's resolved kinds. Run
   schema validation first and abort on failure.
9. **Fixtures and integration test.** Add a kind with a
   schema under `testdata/`, a conformant sample, and
   golden outputs per format. Assert non-conformant input
   exits non-zero with check diagnostics.
10. **Docs.** Add a section under
   [schemas.md](../docs/guides/schemas.md) and a
   `docs/reference/cli/extract.md` page. Both are picked
   up by existing catalog directives. Run `mdsmith fix`
   so catalogs and PLAN.md regenerate.

## Acceptance Criteria

- [ ] `mdsmith extract <kind> --format json <file>` on a
      conformant file emits a tree whose nesting mirrors
      the schema hierarchy, with front matter under
      `frontmatter` — no schema annotations required.
- [ ] Literal headings key by slug; repeating sections
      become arrays; captured placeholders and child
      scopes/content appear as element fields.
- [ ] Code-block, list, table, and paragraph entries
      project under their default keys; sibling key
      collisions are reported as schema diagnostics.
- [ ] A file resolving to multiple kinds yields a merged
      tree; a genuine shape divergence is reported, not
      silently dropped.
- [ ] `yaml`, `lua`, and `msgpack` produce equivalent
      data; golden fixtures cover all four formats.
- [ ] A non-conformant file makes `extract` exit non-zero
      and print the same diagnostics as `mdsmith check`.
- [ ] An unknown kind, or a kind not assigned to the
      file, exits non-zero with a clear message.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes

## Open questions

- Repeating-scope array key: slug of the literal stem vs.
  the placeholder name. Plan assumes literal stem, else
  placeholder name.
- Custom bindings (rename/restructure) ship in [plan
  164](164_custom-binding-overrides.md), layered on the
  `keyFor` seam; out of scope here.
- Lua output: bare `return { … }` table to start, not a
  named module.
- Exposing extraction over the LSP or a `query`-style
  selector is out of scope here.
