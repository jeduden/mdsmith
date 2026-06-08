---
id: 241
title: Schema-per-file config under `.mdsmith/schemas/`
status: "🔲"
model: opus
depends-on: [146, 208, 209]
summary: >-
  Lift reusable schemas out of
  `kinds.<name>.schema:` in `.mdsmith.yml` into
  standalone YAML files under
  `.mdsmith/schemas/<name>.yaml`. A kind references
  one by name (`schema: rfc-v1`) so the same schema
  can drive many kinds without duplication. Also
  closes the conventions docs gap with
  `docs/guides/conventions.md`.
---
# Schema-per-file config under `.mdsmith/schemas/`

## ...

<?allow-empty-section?>

## Goal

A user with many kinds outgrows inline schemas.
This plan adds `.mdsmith/schemas/`: one file per
schema, referenced from any kind by name
(`schema: rfc-v1`). The same schema can then drive
multiple kinds.

## ...

<?allow-empty-section?>

## Background

Today a kind embeds its schema inline under
`kinds.<name>.schema:` or points at a `proto.md`.
Inline does not share; `proto.md` shares but uses
a different matcher grammar. A YAML file under
`.mdsmith/schemas/` shares AND uses the plan 146
matcher engine via
[`schema.ParseInline`](../internal/schema/parse_inline.go).

`docs/guides/conventions.md` does not exist; the
plan adds it as a side deliverable parallel to
[`file-kinds.md`](../docs/guides/file-kinds.md).

## Non-Goals

- Migrating `proto.md` to YAML.
- Removing inline `schema:` maps.
- Schema-file-level `extends:`.
- Adopting `.mdsmith/schemas/` in this repo
  (deferred until the pinned mdsmith version
  ships the feature).
- Auto-binding by basename.

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

The body parses through `schema.ParseInline`.
Allowed top-level keys (anything else errors):
`frontmatter`, `filename`, `closed`, `sections`,
`cross-references`, `acronyms`, and `index`.

```yaml
# .mdsmith/schemas/rfc-v1.yaml
filename: "RFC-[0-9][0-9][0-9][0-9].md"
frontmatter:
  id: '=~"^RFC-[0-9]{4}$"'
  status: '"draft" | "ratified" | "deprecated"'
closed: true
sections:
  - heading: null
  - heading: "Overview"
  - heading: "Decision"
  - heading: "References"
```

The schema's name is the basename minus
extension, matching `[a-z][a-z0-9-]*` — the same
rule kind and convention files carry.

### Referencing from a kind

A kind references a YAML schema file by name
through the existing `schema:` key — its value
becomes polymorphic:

```yaml
kinds:
  rfc:
    schema: rfc-v1            # resolves .mdsmith/schemas/rfc-v1.yaml
  rfc-internal:
    schema: rfc-v1            # one schema drives both kinds
  draft:
    schema:                   # inline form (unchanged)
      filename: "DRAFT-*.md"
```

The string form looks the schema up by name.
The map form keeps its inline meaning. The
merge layer resolves the named reference at
config-load time. The resolved body threads
through the same `schema-sources` plumbing
inline schemas use today.

An undeclared name errors, naming the kind and
the missing schema.

Interactions:

- **`extends:`** — name references resolve to
  bodies **before** `ResolveKindInlineSchema`
  walks the chain, so parent and child schemas
  merge via the plan 135 semantics either way.
- **`path-pattern:`** — runs independently from
  the resolved schema's `filename:`. Both fire
  on a mismatch (unchanged from inline).
- **Namespace** — schema names live in a flat
  registry under `.mdsmith/schemas/`. A `proto.md`
  path under `rules.required-structure.schema:`
  is a separate source type.

### Mutual exclusion

A named `schema:` resolves to an inline map
before validation runs.
[`validateKindSchemaSources`](../internal/config/validate.go)
then rejects any kind with two schema sources,
the same as today.

### Sharing and provenance

Multi-kind composition per
[plan 156](156_kind-schema-composition.md) is
unchanged. Each kind adds one `schema-sources`
entry (`inline` = resolved YAML). `schema.Compose`
merges them.

`applyInlineSchemaSource` already takes a
`sourcePath`. For a named reference it carries
the schema file's path, so a violation's "go to
schema" location points there.

## ...

<?allow-empty-section?>

## Tasks

Per
[Go architecture patterns](../docs/development/architecture/go.md):
no new package. Discovery lives in
`internal/config`.

1. **`internal/config`** (+ `internal/kindsout`):
   add a `SchemaRef` type with `UnmarshalYAML`
   dispatching on `yaml.Node.Kind` (scalar →
   named ref, mapping → inline body). Replace
   `KindBody.Schema map[string]any` and route
   every map reader through a `Map()` accessor:
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
3. **`internal/config`**: add `Schemas
   map[string]map[string]any` on `Config`.
   `mergeSchemaFiles(cfg, cfgPath)` populates
   it. `ParseBytes` skips disk discovery.
4. **`internal/config`**: in `Load`, after the
   kind/convention merges and before
   `ValidateKinds`, call `mergeSchemaFiles` then
   `resolveNamedSchemas(cfg)`. The resolver
   replaces each kind's named `SchemaRef` with
   the discovered body in place. Undeclared
   names error. Unit tests cover happy path,
   undeclared name, and the inline form
   passing through.
5. **`internal/config`**: adapt
   `validateKindSchemaSources` to read
   `body.Schema.Map()` (filled by resolution).
   A named `schema:` plus
   `rules.required-structure.schema:` — or plus
   `inline-schema:` — then trips the existing
   pairwise checks with "pick one source".
6. **`internal/config`**: thread the schema
   file's path through
   `applyInlineSchemaSource` as `sourcePath`
   when the inline body came from a named
   reference. Provenance test asserts the
   `source` key on the schema-sources entry
   carries `.mdsmith/schemas/<name>.yaml`.
7. **`internal/integration`**: contract test
   covers directory layout, basename rule,
   subdirectory/symlink rejection, both
   dual-source rejections, and undeclared-name
   rejection.
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
    declaring a convention inline, the
    top-level `convention:` selector, layering
    rules over a preset, and the "split into a
    file" recipe. The catalog auto-includes it.
14. **Docs — architecture boundaries**: add a
    row in [cross-system.md][cs] for
    `.mdsmith/schemas/`.
15. **Repo migration**: deferred to a follow-up.

[cs]: ../docs/development/architecture/cross-system.md

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] A kind with `schema: foo` (file) and a
      kind with the same body inline produce the
      same diagnostic messages and anchors on a
      reference doc (the schema-source related
      location differs by design).
- [ ] The integration fixture pair (inline vs
      named) emits matching message+anchor
      streams; two kinds sharing `schema: foo`
      both validate.
- [ ] A kind referencing an undeclared schema
      name errors, naming both.
- [ ] Each of these errors with a clear
      message: basename outside
      `[a-z][a-z0-9-]*`, file in a subdirectory,
      symlink, duplicate basename across
      `.yaml`/`.yml`, unknown top-level key.
- [ ] A kind that sets a named `schema:` and
      `rules.required-structure.schema:` errors;
      same for named `schema:` and
      `inline-schema:`. Both quote "pick one
      source".
- [ ] `mdsmith kinds resolve <file>` prints
      the schema's defining-source path. JSON
      `schema-source-path:` is set for file
      sources, omitted for inline.
- [ ] `docs/reference/schema-files.md` exists.
      [schemas.md](../docs/guides/schemas.md)
      documents all three sources.
- [ ] kind-files.md and cross-system.md each
      reference `.mdsmith/schemas/`.
- [ ] `docs/guides/conventions.md` covers the
      four H2s named in task 13.
- [ ] Unit tests cover `SchemaRef.UnmarshalYAML`,
      `discoverSchemas`, `mergeSchemaFiles`,
      and `resolveNamedSchemas`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` clean.
- [ ] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
