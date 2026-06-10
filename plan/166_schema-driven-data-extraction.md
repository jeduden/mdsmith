---
id: 166
title: Schema-driven data extraction (mdsmith extract)
status: "✅"
model: opus
depends-on: [149]
summary: >-
  Derive a default data tree from the hierarchical
  schema and add an `extract` subcommand that emits a
  kind-conformant file as JSON/YAML/msgpack.
---
# Schema-driven data extraction (mdsmith extract)

## Goal

Let a kind's schema double as an extraction contract.
Once `mdsmith check` confirms a file conforms, `mdsmith
extract <kind> --format json|yaml|msgpack <file>` emits a
data tree. Its shape is derived from the schema hierarchy
itself — no annotations required.

## Why a default binding layer first

The schema is already a hierarchy: front matter, then a
tree of scopes (sections), each with child scopes and
content entries. That hierarchy *is* the data shape. So
the first deliverable is a **default binding layer** that
projects the schema tree into a data tree directly,
mirroring its nesting. No new schema concept is needed
for the common case.

Custom shaping is *not* in this plan. It is a separate
follow-up — [plan 167](167_custom-binding-overrides.md) —
and we keep it cheap by design: every key flows through
one `keyFor(node)` seam (task 3), so the override plan is
a focused change there plus parsing `bind:`. Until then,
renaming or restructuring is the job of a downstream tool
(`jq`, `yq`) over the standard-format output.

## Default projection rules

The projection walks the composed schema in lockstep with
the validated match and mirrors the hierarchy:

- **Root shape.** The root object holds a `frontmatter`
  object (the decoded front matter, unchanged) *and* the
  projected sections beside it at the same level. Front
  matter stays grouped so it never collides with a
  section slug.
- **Literal-heading scope** (`## Goal`) → object keyed by
  the slugified heading (`goal`), reusing the existing
  anchor slugifier. Its value holds child scopes and
  content, recursively.
- **Repeating scope** (`## {id}` with a `repeat: {min,
  max}` cardinality) → an array keyed by the slug of the
  heading's literal stem,
  or the placeholder name if the heading is only a
  placeholder. Each element is an object that **always
  retains every captured placeholder as a `name: value`
  field** (both the placeholder name and its value
  survive), plus the element's own child scopes and
  content.
- **No-heading section** (`heading: null` — content
  before the first child heading) has no heading text and
  therefore no slug. Its content entries project
  **directly into the enclosing object** (root, or the
  parent section) beside the headed-section keys — there
  is no `preamble` wrapper key. Wildcard slots
  (`regex: '.+'`) and unlisted/closed headings are
  skipped: the output is a faithful projection of the
  *declared* schema only.
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

This plan consumes the reworked schema engine, not the
legacy single-source model.

- **Entry-shape unification (`2606022124_schema-entry-unification`
  / PR #295) — landed in main.** Every `sections:` entry
  is discriminated by its `heading:` value: a string or
  `{regex, repeat?, sequential?}` mapping for headed
  sections, and `heading: null` for the no-heading section
  (content before the first child heading). There is no
  standalone `preamble:` key. The projection rules above
  target this shape directly.
- **[Plan 156 — kind-schema
  composition](156_kind-schema-composition.md) / PR
  #288.** (Two plan files share id 156, so this
  dependency is named by filename here rather than in
  numeric `depends-on:`: it is the composition one, not
  the now-landed `2606022124_schema-entry-unification`.) A file
  can resolve to multiple kinds whose schemas compose via
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
   build the nested structure per the rules above:
   `frontmatter` plus sections at the root, literal scopes
   keyed by slug, the `heading: null` no-heading section's
   content hoisted into the enclosing object, wildcard /
   unlisted skipped. Route every key through one
   `keyFor(node)` function — the single seam a future
   custom-binding plan overrides. Reuse the existing
   anchor slugifier. Unit-test literal, nested,
   no-heading-section, and optional-omitted scopes.
4. **Repeating scopes and placeholders.** Project scopes
   with a `repeat: {min, max}` cardinality as arrays; each
   element retains
   every captured `{field}` as a `name: value` field,
   reusing
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
   with json (stdlib), yaml (existing dep), and msgpack
   encoders behind a `Format` enum. (Lua is deferred.)
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

- [x] `mdsmith extract <kind> --format json <file>` on a
      conformant file emits a tree whose nesting mirrors
      the schema hierarchy — no schema annotations
      required.
- [x] The root holds a `frontmatter` object and the
      projected sections beside it at the same level.
- [x] Literal headings key by slug; repeating sections
      become arrays; each element retains every captured
      placeholder as a `name: value` field plus its child
      scopes/content.
- [x] A `heading: null` no-heading section's content
      projects into its enclosing object (no `preamble`
      wrapper key); wildcard and unlisted/closed headings
      are skipped.
- [x] Code-block, list, table, and paragraph entries
      project under their default keys; sibling key
      collisions are reported as schema diagnostics.
- [x] A file resolving to multiple kinds yields a merged
      tree; a genuine shape divergence is reported, not
      silently dropped.
- [x] `json`, `yaml`, and `msgpack` produce equivalent
      data; golden fixtures cover all three formats.
- [x] A non-conformant file makes `extract` exit non-zero
      and print the same diagnostics as `mdsmith check`.
- [x] An unknown kind, or a kind not assigned to the
      file, exits non-zero with a clear message.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] `mdsmith check .` passes

## Decisions

- **Repeating-scope key.** Array key is the slug of the
  heading's literal stem, or the placeholder name when
  the heading is only a placeholder. Each element always
  retains every captured placeholder as a `name: value`
  field, so both the name and the value survive.
- **Front matter placement.** The root holds a
  `frontmatter` object and the projected sections beside
  it at the same level. Grouping front matter avoids
  collisions with section slugs.
- **No-heading section.** A `heading: null` entry has no
  slug; its content projects directly into the enclosing
  object rather than under a `preamble` wrapper key. The
  sibling-collision rule covers any clash with a section
  slug. Wildcard slots and unlisted/closed headings are
  skipped.
- **Lua deferred.** Ship json, yaml, and msgpack. A Lua
  encoder can be added later behind the same `Format`
  enum.
- **Custom bindings** ship in [plan
  167](167_custom-binding-overrides.md), layered on the
  `keyFor` seam; out of scope here.
- **LSP / `query`-style selector** for extraction is out
  of scope here.
