---
id: 241
title: Schema-per-file config under `.mdsmith/schemas/`
status: "🔲"
model: opus
depends-on: [146, 156, 208, 209]
summary: >-
  Add a top-level `schemas:` registry to
  `.mdsmith.yml`, mirrored at
  `.mdsmith/schemas/<name>.yaml` — the same
  pattern plans 208 and 209 set for `kinds:` and
  `conventions:`. A kind references a schema by
  name (`schema: rfc-v1`). Also adds the missing
  `docs/guides/conventions.md` guide.
---
# Schema-per-file config under `.mdsmith/schemas/`

## ...

<?allow-empty-section?>

## Goal

Plans 208 and 209 let a crowded `kinds:` or
`conventions:` block move into one file per entry
under `.mdsmith/kinds/` and `.mdsmith/conventions/`.
Inline schemas have the same problem and no such
escape. This plan adds a top-level `schemas:`
registry mirrored at `.mdsmith/schemas/<name>.yaml`,
so each schema moves to its own file. A kind
references one by name (`schema: rfc-v1`); one
schema can drive several kinds.

## ...

<?allow-empty-section?>

## Background

Today a kind embeds its schema inline under
`kinds.<kind>.schema:` or points at a `proto.md`.
Inline doesn't share; `proto.md` shares but uses
a different matcher grammar. The registry
shares AND uses the plan 156 matcher engine via
[`schema.ParseInline`](../internal/schema/parse_inline.go).

`docs/guides/conventions.md` does not exist; the
plan adds it as a side deliverable.

## Non-Goals

- Migrating `proto.md` to YAML.
- Removing inline `schema:` maps on kinds.
- Schema-file-level `extends:`.
- Adopting `.mdsmith/schemas/` in this repo
  (deferred until the pinned mdsmith ships it).

## Design

### Directory layout

```text
.mdsmith.yml                       # unchanged
.mdsmith/
  kinds/                           # plan 208
  conventions/                     # plan 209
  schemas/
    rfc-v1.yaml
    runbook.yaml
```

Both `*.yaml` and `*.yml` are scanned.
Subdirectories and symlinks are rejected. The
1 MB `.mdsmith.yml` cap applies unchanged.

### Schema file shape

A file `.mdsmith/schemas/<name>.yaml` is the
per-file split of an inline `schemas.<name>:`
block. The body parses through
`schema.ParseInline`. Top-level keys allowed:

- `frontmatter`
- `filename`
- `closed`
- `sections`
- `cross-references`
- `acronyms`
- `index`

The basename is the schema's name, matching
`[a-z][a-z0-9-]*` — same rule kind and convention
files carry.

### Referencing from a kind

A kind's `schema:` becomes polymorphic: a string
is a registry name, a map stays inline.

```yaml
# .mdsmith.yml
schemas:                    # inline equivalent of
  rfc-v1:                   # .mdsmith/schemas/rfc-v1.yaml
    filename: "RFC-[0-9][0-9][0-9][0-9].md"
    sections:
      - heading: "Overview"
      - heading: "Decision"

kinds:
  rfc:
    schema: rfc-v1          # registry reference
  rfc-internal:
    schema: rfc-v1          # one schema drives both kinds
  draft:
    schema:                 # inline body still allowed
      filename: "DRAFT-*.md"
```

The merge layer resolves the registry reference
at load time so the rule sees one inline body.
An undeclared name errors. A name declared both
inline under `schemas:` AND as a file errors,
naming both — same rule as kinds and conventions.

Interactions:

- **`extends:`** — registry refs resolve to
  bodies **before** `ResolveKindInlineSchema`
  walks the chain.
- **`path-pattern:`** — runs independently from
  the resolved schema's `filename:`. Both fire
  on a mismatch.

### Mutual exclusion

A named `schema:` resolves to a map before
[`validateKindSchemaSources`](../internal/config/validate.go)
runs, so a two-source kind is rejected as today.
A referenced schema's body is grammar-checked
when its kind validates — the same point an
inline schema is checked today. An undeclared
name errors at load.

### Provenance

Composition per
[plan 156](156_kind-schema-composition.md) is
unchanged: each kind adds one `schema-sources`
entry, merged by `schema.Compose`.

`KindSchemaRef` carries the schema's own
`SourcePath`, set at resolution. It is the `.yaml`
path for a file entry, `.mdsmith.yml` for an
inline-registry entry, empty for an inline-on-kind
body (the kind's file then applies).
`applyInlineSchemaSource` threads it, so "go to
schema" lands on the schema, not the kind.

## ...

<?allow-empty-section?>

## Tasks

Per
[Go architecture patterns](../docs/development/architecture/go.md):
no new package; the type change ripples from
`internal/config` into `internal/kindsout`.

1. **`internal/config`** (+ `internal/kindsout`):
   add a `KindSchemaRef` type with `UnmarshalYAML`
   dispatching on `yaml.Node.Kind` (scalar →
   named ref, mapping → inline body) plus a
   `SourcePath` field for the schema's own origin.
   Replace `KindBody.Schema map[string]any` and
   route every map reader through a `Map()`
   accessor:
   `resolvedInlineSchema`, `extendsChainSchemas`,
   `validateKindSchemaSources`,
   `effectiveExplicit`, `resolveLayerInlineSchema`,
   and `kindsout`'s frontmatter index.
   `copyKinds` must deep-copy the resolved body
   so load-time resolution survives the merge.
   Unit test covers scalar, mapping, sequence,
   null, malformed scalar.
2. **`internal/config`**: add
   `discoverSchemas(workspaceDir)` mirroring
   `discoverKinds`'s checks (basename
   `[a-z][a-z0-9-]*`, no subdirs, no symlinks,
   no `.yaml`/`.yml` duplicates, no unknown
   top-level keys). Unit test per rejection.
3. **`internal/config`**: store the registry as
   `map[string]discoveredSchema` (`{body,
   sourcePath}`) so each entry keeps its origin.
   Inline `schemas:` entries are tagged
   `.mdsmith.yml`; `mergeSchemaFiles` tags file
   entries with the `.yaml` path and errors on an
   inline-vs-file collision. `ParseBytes` skips
   disk discovery.
4. **`internal/config`**: in `Load`, after the
   kind/convention merges and before
   `ValidateKinds`, call `mergeSchemaFiles` then
   `resolveNamedSchemas(cfg)`. The resolver
   replaces each kind's named `KindSchemaRef` with
   the discovered body and sets the ref's
   `SourcePath` from the entry's origin. An
   undeclared name errors. Unit tests cover happy
   path, undeclared name, inline passing through.
5. **`internal/config`**: adapt
   `validateKindSchemaSources` to read
   `body.Schema.Map()` (filled by resolution).
   A named `schema:` plus
   `rules.required-structure.schema:` — or plus
   `inline-schema:` — then trips the existing
   pairwise checks with "pick one source".
6. **`internal/config`**: thread the ref's
   `SourcePath` through `applyInlineSchemaSource`,
   falling back to the kind's file when empty
   (inline-on-kind). Provenance tests assert the
   `source` key is `.mdsmith/schemas/<name>.yaml`
   for a file entry, `.mdsmith.yml` for an
   inline-registry entry.
7. **`internal/integration`**: contract test
   covers directory layout, basename rule,
   subdirectory/symlink rejection, the two
   dual-source rejections, inline-vs-file and
   cross-extension collisions, undeclared names.
8. **`internal/integration`**: parallel
   fixtures — one kind defined inline, one via
   named YAML reference, same Markdown input —
   assert byte-equal diagnostic streams.
9. **CLI**: extend `mdsmith kinds resolve` to
   print the schema's defining-source path.
   JSON gains `schema-source-path:`, populated
   for named-YAML and `proto.md` sources,
   omitted for inline (already on `source-path`).
10. **Docs — new reference**: add
    `docs/reference/schema-files.md` with the
    H2s kind-files.md and convention-files.md
    share (file shape, basename rule,
    composition, audit); the directory tree
    goes in the preamble.
11. **Docs — schemas guide**: in
    [schemas.md](../docs/guides/schemas.md)
    grow the source list to three, add a
    "File-based YAML schemas" H2 between inline
    and proto.md, and add a column to the
    "Choosing a source" table.
12. **Docs — kind-files reference**: in
    [kind-files.md](../docs/reference/kind-files.md)'s
    "Schema sources" section, add the named
    reference as a fourth source.
13. **Docs — conventions guide**: add
    `docs/guides/conventions.md` with H2s for
    built-in vs user conventions, declaring one
    inline, the `convention:` selector, the
    flavor-must-agree rule, layering rules over
    a preset, and the "split into a file"
    recipe. `mdsmith fix` regenerates the
    CLAUDE.md catalog to include it.
14. **Docs — architecture boundaries**: add a
    row in [cross-system.md][cs] for
    `.mdsmith/schemas/`.
15. **Repo migration**: deferred to a follow-up.

[cs]: ../docs/development/architecture/cross-system.md

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] A `schema: foo` (file) kind and a kind
      with the same body inline produce the same
      messages and anchors on a reference doc;
      only the schema-source location differs.
- [ ] The inline-vs-named fixture pair emits
      matching message+anchor streams; two kinds
      sharing `schema: foo` both validate.
- [ ] An undeclared `schema:` name errors,
      naming the kind and the missing schema.
- [ ] Each of these errors with a clear
      message: basename outside
      `[a-z][a-z0-9-]*`, file in a subdirectory,
      symlink, duplicate basename across
      `.yaml`/`.yml`, unknown top-level key,
      name declared inline under `schemas:` AND
      as a file.
- [ ] A kind that sets a named `schema:` and
      `rules.required-structure.schema:` errors;
      same for named `schema:` and
      `inline-schema:`. Both quote "pick one
      source".
- [ ] `mdsmith kinds resolve <file>` prints the
      schema's source path; `schema-source-path:`
      is set for file and inline-registry sources,
      omitted for inline-on-kind.
- [ ] `docs/reference/schema-files.md` exists.
      [schemas.md](../docs/guides/schemas.md)
      documents all three sources.
- [ ] kind-files.md and cross-system.md each
      reference `.mdsmith/schemas/`.
- [ ] `docs/guides/conventions.md` covers the
      H2s named in task 13.
- [ ] Unit tests cover `KindSchemaRef.UnmarshalYAML`,
      `discoverSchemas`, `mergeSchemaFiles`,
      and `resolveNamedSchemas`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` clean.
- [ ] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
