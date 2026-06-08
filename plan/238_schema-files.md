---
id: 238
title: Schema-per-file config under `.mdsmith/schemas/`
status: "🔲"
model: opus
depends-on: [146, 208]
summary: >-
  Lift reusable schemas out of
  `kinds.<name>.schema:` in `.mdsmith.yml` into
  standalone YAML files under
  `.mdsmith/schemas/<name>.yaml`. A kind references
  one by name (`schema: rfc-v1`) so the same schema
  can drive many kinds without duplication. Inline
  `schema:` maps stay first-class. Also closes the
  conventions docs gap by adding
  `docs/guides/conventions.md`.
---
# Schema-per-file config under `.mdsmith/schemas/`

## ...

<?allow-empty-section?>

## Goal

Lift each reusable schema out of
`kinds.<name>.schema:` into a standalone YAML file
under `.mdsmith/schemas/<name>.yaml`, addressable
by name from any kind. One file holds one
schema's body so the same heading tree can drive
multiple kinds without copy-paste.

## ...

<?allow-empty-section?>

## Background

Plan 208 split kinds into
[`.mdsmith/kinds/`](../docs/reference/kind-files.md).
Plan 209 split user conventions into
[`.mdsmith/conventions/`](../docs/reference/convention-files.md).
Schemas are the next named, reusable bundle.

Today a kind either embeds its schema inline
under `kinds.<name>.schema:` or points at a
`proto.md` file. The inline form does not share.
The `proto.md` form shares but does not use the
modern matcher engine from plan 146 (`regex:`,
`repeat:`, `\#(digits)`, `\#(fmvar(...))`).

A YAML file under `.mdsmith/schemas/` closes
the gap. The body parses through the existing
[`schema.ParseInline`](../internal/schema/parse_inline.go).
Kinds reference it by name; the same schema may
drive many kinds.

The conventions surface has a docs gap too:
no `docs/guides/conventions.md` parallel to
[`docs/guides/file-kinds.md`](../docs/guides/file-kinds.md).
This plan closes that gap as a side
deliverable.

## Non-Goals

- Migrating `proto.md` to YAML. `proto.md`
  stays first-class.
- Removing inline `schema:` maps. Inline stays
  first-class.
- Schema-file-level `extends:`. Out of scope.
- Adopting `.mdsmith/schemas/` in this repo.
  Deferred until the pinned mdsmith version
  ships the feature.
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
workspace root. Subdirectories and symlinks
under `.mdsmith/schemas/` are rejected. The 1 MB
size cap that protects `.mdsmith.yml` applies
unchanged.

### Schema file shape

The body matches the inline
`kinds.<name>.schema:` shape and feeds into
`schema.ParseInline` unchanged:

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
extension. The basename must match
`[a-z][a-z0-9-]*` — the same rule kind and
convention files carry. A top-level key outside
the inline-schema vocabulary errors naming the
key.

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
config-load time and threads the resolved body
through the same `schema-sources` plumbing
inline schemas use today.

An undeclared name is a config error naming
the kind and the missing schema.

### Mutual exclusion

[`validateKindSchemaSources`](../internal/config/validate.go)
already rejects a kind that sets more than one
of `kinds.<name>.schema:`,
`rules.required-structure.schema:`, or the
legacy `inline-schema:`. The named form is a
fourth shape on the inline slot. The check
treats it identically to the inline map for
the pairwise comparison.

### Sharing across kinds

A file resolving to several kinds per
[plan 156](156_kind-schema-composition.md)
where each kind references the same schema
name composes identically to the inline case.
The existing `schema.Compose` deduplicates
byte-equal sources. No new composition
semantics.

## ...

<?allow-empty-section?>

## Tasks

Per
[Go architecture patterns](../docs/development/architecture/go.md):
no new package. Discovery lives in
`internal/config`.

1. **`internal/config`**: add a `SchemaRef`
   type wrapping the polymorphic `schema:`
   value with `UnmarshalYAML` that dispatches
   on `yaml.Node.Kind`. Replace
   `KindBody.Schema map[string]any` with the
   new type. Unit test covers scalar,
   mapping, and error paths.
2. **`internal/config`**: add
   `discoverSchemas(workspaceDir)` modelled on
   `discoverKinds`. Rejects bad basenames,
   subdirectories, symlinks, duplicates, and
   unknown top-level keys. Unit test per
   rejection case.
3. **`internal/config`**: add `Schemas` map
   on `Config`. `mergeSchemaFiles(cfg,
   cfgPath)` populates it. The in-memory
   `ParseBytes` path skips disk discovery.
4. **`internal/config`**: in `Load`, after
   the kind and convention merges, call
   `mergeSchemaFiles` and then
   `resolveNamedSchemas(cfg)`. Replaces each
   named reference with the resolved inline
   body. Undeclared names error. Unit test
   covers happy path, undeclared name, and
   inline form passing through.
5. **`internal/config`**: extend
   `validateKindSchemaSources` so a kind that
   sets both a named `schema:` and
   `rules.required-structure.schema:` (or
   `inline-schema:`) errors with the existing
   "pick one source" wording.
6. **`internal/config`**: extend provenance
   so a resolved named schema records the
   schema file's path as its source.
7. **`internal/integration`**: contract test
   covers directory layout, basename rule,
   subdirectory rejection, dual-source
   rejection, and undeclared-name rejection.
8. **`internal/integration`**: parallel
   fixtures asserting a kind referencing a
   YAML schema produces the same diagnostics
   as the equivalent inline schema.
9. **CLI**: extend `mdsmith kinds resolve` to
   print the schema's defining-source path.
   JSON shape gains `schema-source-path:`.
10. **Docs — new reference**: add
    `docs/reference/schema-files.md` modelled
    on `kind-files.md` and
    `convention-files.md`.
11. **Docs — schemas guide**: extend
    [schemas.md](../docs/guides/schemas.md)
    with a "File-based YAML schemas" section.
    Update the source list and the "Choosing
    a source" table.
12. **Docs — kind-files reference**: note the
    named reference as a third schema source
    in
    [kind-files.md](../docs/reference/kind-files.md).
13. **Docs — conventions guide**: add
    `docs/guides/conventions.md` mirroring
    `file-kinds.md`. Wire it into the
    [CLAUDE.md](../CLAUDE.md) catalog.
14. **Docs — architecture boundaries**: add a
    row to the boundaries table in
    [cross-system.md][cs] listing
    `.mdsmith/schemas/`.
15. **Repo migration**: deferred until the
    pinned mdsmith version supports the
    directory.

[cs]: ../docs/development/architecture/cross-system.md

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] A schema at `.mdsmith/schemas/foo.yaml`
      whose body matches an inline
      `kinds.bar.schema:` produces byte-equal
      diagnostics on a reference document
      when the kind sets `schema: foo`.
- [ ] Two kinds referencing the same schema
      name both validate against it.
- [ ] A kind setting `schema: <name>` where
      the name is undeclared errors, naming
      the kind and the missing schema.
- [ ] A schema basename failing
      `[a-z][a-z0-9-]*`, a file in a
      subdirectory, or a symlink errors.
- [ ] Two schema files with the same
      basename across `.yaml`/`.yml` error
      naming both files.
- [ ] A schema file with an unknown
      top-level key errors naming the key.
- [ ] A kind setting both a named `schema:`
      and `rules.required-structure.schema:`
      errors with the "pick one source"
      wording.
- [ ] `mdsmith kinds resolve <file>` prints
      the schema's defining-source path;
      JSON carries `schema-source-path:`.
- [ ] `docs/reference/schema-files.md`
      exists and is linked from the catalog,
      the schemas guide, and the boundaries
      table.
- [ ] [schemas.md](../docs/guides/schemas.md)
      documents all three sources.
- [ ] `docs/guides/conventions.md` exists
      and mirrors
      [file-kinds.md](../docs/guides/file-kinds.md).
- [ ] Every new function in
      `internal/config` ships with its
      dedicated unit test.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` clean.
- [ ] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
