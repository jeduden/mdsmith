---
id: 238
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

Lift each reusable schema out of
`kinds.<name>.schema:` into a standalone YAML file
under `.mdsmith/schemas/<name>.yaml`. Any kind
references it by name via `schema: rfc-v1`.

## ...

<?allow-empty-section?>

## Background

Plans 208 and 209 split kinds and conventions
into `.mdsmith/kinds/` and
`.mdsmith/conventions/`. Schemas are the next
named, reusable bundle.

Today a kind embeds its schema inline under
`kinds.<name>.schema:` or points at a `proto.md`.
Inline does not share; `proto.md` shares but
uses a different matcher grammar. A YAML file
under `.mdsmith/schemas/` shares AND uses the
plan 146 matcher engine. The body parses through
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

Both `*.yaml` and `*.yml` are scanned at the
workspace root. Subdirectories and symlinks are
rejected. The 1 MB cap on `.mdsmith.yml` applies
unchanged.

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

[`validateKindSchemaSources`](../internal/config/validate.go)
rejects a kind that sets more than one of
`kinds.<name>.schema:`,
`rules.required-structure.schema:`, or the
legacy `inline-schema:`. The named form is a new
shape on the `schema:` slot. The check treats it
like the inline map for both pairwise checks.

### Sharing and provenance

Multi-kind composition per
[plan 156](156_kind-schema-composition.md) is
unchanged. Each kind contributes one
`schema-sources` entry whose `inline` body is
the resolved YAML; `schema.Compose` merges them.

`applyInlineSchemaSource` in
[`merge.go`](../internal/config/merge.go)
already accepts a `sourcePath`. For a named
reference it carries the schema file's path; for
the inline form, the kind's defining file. The
entry's `source` key holds it so diagnostics
navigate to the right file.

## ...

<?allow-empty-section?>

## Tasks

Per
[Go architecture patterns](../docs/development/architecture/go.md):
no new package. Discovery lives in
`internal/config`.

1. **`internal/config`**: add a `SchemaRef`
   type with `UnmarshalYAML` dispatching on
   `yaml.Node.Kind`. Scalar string → named
   reference; mapping → inline body. Replace
   `KindBody.Schema map[string]any` with it.
   Unit test covers scalar, mapping, sequence,
   null, and malformed scalar.
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
   kind and convention merges, call
   `mergeSchemaFiles` and then
   `resolveNamedSchemas(cfg)`. The resolver
   replaces each kind's named `SchemaRef` with
   the discovered body in place. Undeclared
   names error. Unit tests cover happy path,
   undeclared name, and the inline form
   passing through.
5. **`internal/config`**: extend
   `validateKindSchemaSources` so a kind
   setting a named `schema:` AND
   `rules.required-structure.schema:` errors,
   and so a kind setting a named `schema:` AND
   `rules.required-structure.inline-schema:`
   errors — both with the "pick one source"
   wording.
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
   print the schema's defining-source path on
   the kind's resolved entry. JSON shape gains
   `schema-source-path:`, omitted when the
   schema is inline (its source is already on
   `source-path`).
10. **Docs — new reference**: add
    `docs/reference/schema-files.md` with the
    H2s kind-files.md and convention-files.md
    share: directory layout, file shape,
    basename rule, composition with
    `.mdsmith.yml`, audit surface.
11. **Docs — schemas guide**: in
    [schemas.md](../docs/guides/schemas.md)
    update the source list to three sources,
    add a "File-based YAML schemas" H2 between
    inline and proto.md, and add a row to the
    "Choosing a source" table.
12. **Docs — kind-files reference**: in
    [kind-files.md](../docs/reference/kind-files.md)'s
    "Schema sources" section, add the named
    reference as a fourth source.
13. **Docs — conventions guide**: add
    `docs/guides/conventions.md` with H2s:
    declaring a convention inline, picking
    one via top-level `convention:`, layering
    rules over a preset, and the "split into
    its own file" recipe. The CLAUDE.md
    catalog picks it up on next `mdsmith fix`.
14. **Docs — architecture boundaries**: add a
    row in [cross-system.md][cs] for
    `.mdsmith/schemas/`.
15. **Repo migration**: deferred to a follow-up.

[cs]: ../docs/development/architecture/cross-system.md

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] A schema at `.mdsmith/schemas/foo.yaml`
      with the body of inline
      `kinds.bar.schema:` produces byte-equal
      diagnostics when the kind sets
      `schema: foo`.
- [ ] Two kinds with `schema: foo` produce
      identical diagnostics; the integration
      fixture pair (inline vs named) emits
      byte-equal streams.
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
      adds `schema-source-path:`, omitted when
      the schema is inline.
- [ ] `docs/reference/schema-files.md` exists.
      [schemas.md](../docs/guides/schemas.md)
      documents all three sources.
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
