---
id: 163
title: Schema-driven data extraction (mdsmith extract)
status: "🔲"
model: opus
depends-on: [149, 156]
summary: >-
  Add a bind layer to schemas and an `extract`
  subcommand that turns a kind-conformant Markdown
  file into JSON/YAML/Lua/msgpack following the
  schema.
---
# Schema-driven data extraction (mdsmith extract)

## Goal

Let a kind's schema double as an extraction contract.
Once `mdsmith check` confirms a file conforms, `mdsmith
extract <kind> --format json|yaml|lua|msgpack <file>`
emits a data tree. Its shape is exactly what the schema
declares.

## Why a new concept is needed

Today the schema engine
([internal/schema](../internal/schema)) validates
*structure*: the heading tree, repeat cardinality,
content-node kinds, and front-matter CUE constraints.
But its [Scope](../internal/schema/schema.go) and
`ContentEntry` nodes are **anonymous**. The matcher knows
a `## {id}` heading must exist, yet not what key `id`
becomes in output, and it never captures node bodies.
Front matter is already YAML-typed and passes through
unchanged. The gap is the document body.

So this plan adds exactly one new schema concept — a
**binding / projection layer** — plus a thin extractor
that walks the existing validated match. No second
parser, and no schema-to-type inference.

## Sequencing

The schema engine is mid-rework. This plan must land
after, and consume the outputs of, that work rather than
the legacy single-source model.

- **Plan 156 / PR #288 (schema composition).** A file can
  resolve to multiple kinds whose schemas compose via
  `schema.Compose()`. The extractor consumes the composed
  `Schema`, never a single `Rule.Schema`. This forces a
  new rule: `bind:` names must compose too. Identical
  headings from different kinds merge their bound
  children. Two kinds binding the same node to different
  names is a schema error raised at compose time.
- **Plan 149 (section-content schema).** Body extraction
  rides on the `ContentEntry` model from the
  content-schema work. This plan adds no content matcher
  of its own and is blocked until that model is stable.
- **Plan 147 / PR #284 (actionable schema diagnostics).**
  If landed, bind validation and conformance failures
  reuse the `SchemaDiagnostic` formatter.

## The `bind:` concept

`bind:` is an optional string key on any schema scope or
content entry. It names the value that node contributes
to the extracted tree. Unbound nodes are structural only
and emit nothing, keeping output intentional.

Mapping rules:

- **Front matter** → top-level `frontmatter` object,
  passed through from the existing decode.
- **Non-repeating scope, `bind: x`** → object `x` holding
  its bound children.
- **Repeating scope, `bind: xs`** (a `{placeholder}`
  heading) → array `xs`. Each element is an object whose
  fields are the captured placeholders plus bound
  children.
- **`code-block`, `bind: c`** → string `c` (raw body);
  optional `parse: yaml|json` embeds the decoded value.
- **`list`, `bind: items`** → array of item strings.
- **`table` with `columns`, `bind: rows`** → array of row
  objects keyed by column header.
- **`paragraph`, `bind: t`** → its text.

Extraction is gated on a successful schema match. A
non-conformant file makes `extract` report the same
diagnostics as `check` and exit non-zero. It never emits
partial data.

## Tasks

1. **Parse `bind:` (red/green).** Add a `Bind string`
   field to `Scope` and `ContentEntry` in
   [schema.go](../internal/schema/schema.go); parse it in
   [parse_inline.go](../internal/schema/parse_inline.go)
   and [parse_file.go](../internal/schema/parse_file.go).
   Unit-test round-trip and that an empty bind means
   "structural only".
2. **Validate `bind:` names.** Reject duplicate sibling
   binds, and binds whose value is unreachable because a
   parent is unbound. Surface via the `SchemaDiagnostic`
   path (plan 147) if landed, else a plain parse error.
3. **Compose `bind:` across kinds.** Extend
   `schema.Compose()` (plan 156) so merged headings union
   their bound children. Binding one composed node to two
   different names is a compose-time error. Add
   `compose_test.go` cases for union and conflict.
4. **Extractor package.** Add `internal/extract` with
   `Extract(f *lint.File, sch *schema.Schema, match …)
   (any, []lint.Diagnostic)`. `sch` is the composed
   schema. It consumes the existing schema-validation
   walk (extend `schema.Validate` / the content matcher
   to expose the scope→nodes match tree) rather than
   re-matching.
5. **Capture placeholder values.** When a repeating scope
   matches a `{field}` heading pattern, record each
   captured field into the element object, reusing
   [fieldinterp](../internal/fieldinterp/fieldinterp.go).
6. **Format encoders.** Add `internal/extract/encode`
   with json (stdlib), yaml (existing dep), msgpack, and
   lua (table literal) encoders behind a `Format` enum.
7. **`extract` subcommand.** Register `extract` in
   [main.go](../cmd/mdsmith/main.go); signature `mdsmith
   extract <kind> --format <fmt> <file>`. Reuse the
   config-load and kind-resolution helpers from
   [kinds.go](../cmd/mdsmith/kinds.go). Validate that
   `<kind>` is one of the file's resolved kinds. Run
   schema validation first and abort on failure.
8. **Fixtures and integration test.** Add a kind with a
   bound schema under `testdata/`, a conformant sample,
   and golden outputs per format. Assert non-conformant
   input exits non-zero with check diagnostics.
9. **Docs.** Add a section under
   [schemas.md](../docs/guides/schemas.md) and a
   `docs/reference/cli/extract.md` page. Both are picked
   up by existing catalog directives. Run `mdsmith fix`
   so catalogs and PLAN.md regenerate.

## Acceptance Criteria

- [ ] `bind:` parses on scopes and content entries from
      inline and `proto.md` schemas. Duplicate and
      unreachable binds are rejected with actionable
      diagnostics.
- [ ] `mdsmith extract <kind> --format json <file>` on a
      conformant file emits a tree matching the bound
      schema, with front matter under `frontmatter`.
- [ ] A file resolving to multiple kinds composes its
      binds; conflicting binds on one composed node are a
      compose-time error.
- [ ] Repeating sections become arrays. Placeholder
      captures and bound children appear as element
      fields. Code-block, list, table, and paragraph
      binds extract as specified.
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

- Should `bind:` default to the slugified heading text,
  or stay strictly opt-in? The plan assumes opt-in.
- Lua output: bare `return { … }` table to start, not a
  named module.
- Exposing extraction over the LSP or a `query`-style
  selector is out of scope here.
