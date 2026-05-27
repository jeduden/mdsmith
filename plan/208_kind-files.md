---
id: 208
title: Kind-per-file config under `.mdsmith/kinds/`
status: "✅"
model: opus
depends-on: [146, 135]
summary: >-
  Move a kind out of `.mdsmith.yml` into a
  standalone YAML file whose basename is the
  kind name. The file holds the full
  `KindBody` (schema, rules, path-pattern,
  extends). The same `.mdsmith/` directory
  reserves slots for conventions to follow in
  a later plan.
---
# Kind-per-file config under `.mdsmith/kinds/`

## Goal

Lift each kind out of `.mdsmith.yml` into a
standalone YAML file. The basename is the
kind's identity. The file body holds the
complete kind — schema, rules, path-pattern,
extends — so one file describes everything
about that kind in one place.

## Background

A kind today lives under `kinds.<name>:` in
`.mdsmith.yml` (plan 92). Its body — schema,
rules, `path-pattern:`, `extends:`,
`categories:` — sits in the one shared file.

This repo's `.mdsmith.yml` is 558 lines; the
`kinds:` block alone covers lines 295–480.
A change to one kind dirties the same file
as every other config edit, and reading one
kind means scrolling past a dozen others.

A YAML file per kind (basename = name)
makes each self-contained. The `.mdsmith/`
tree reserves a `conventions/` slot for
later.

## Non-Goals

- Removing inline `kinds.<name>:` from
  `.mdsmith.yml`. Inline stays as a
  first-class source.
- Migrating `proto.md` schemas to YAML;
  `proto.md` stays. A kind file may still
  set `rules.required-structure.schema:`
  pointing at a `proto.md`.
- A standalone-schema directory. A schema
  shared across kinds is shared via
  `extends:` (plan 135), not by extracting
  the `schema:` block to its own file.
- Externalising conventions. The
  `.mdsmith/conventions/` slot is reserved
  but not wired in this plan.
- Schema versioning (V-1).

## Design

### Directory layout

```text
.mdsmith.yml                   # unchanged
.mdsmith/
  kinds/
    audit-log.yaml             # one full kind
    secret-rotation.yaml
    architecture-doc.yaml
```

The discovery root is the workspace root
(parent of `.mdsmith.yml`). Both `*.yaml`
and `*.yml` are scanned. Subdirectories
under `.mdsmith/kinds/` are rejected at load
time.

### Kind file shape

The body matches today's inline
`kinds.<name>:` shape. It holds the full
`KindBody` keys: `extends:`, `path-pattern:`,
`categories:`, `schema:`, `rules:`.

```yaml
# .mdsmith/kinds/audit-log.yaml
schema:
  frontmatter:
    title: 'string & != ""'
    "summary?": 'string'
    audit-from: '=~"^[0-9a-f]{7,40}$"'
  filename: "architecture-audit.md"
  closed: false
  sections:
    - heading: null
    - heading:
        regex: '.+'
        repeat: { min: 0 }
rules:
  max-file-length:
    max: 600
```

The kind's name is the basename minus
extension. The basename must match
`[a-z][a-z0-9-]*` — a new constraint added
here for filenames (OS case folding, path
syntax) that doesn't apply to inline YAML
keys. A bad basename errors. Inline
`kinds.<name>:` keys stay unvalidated.

One kind per file. A YAML document with
top-level keys outside the `KindBody`
schema is a config error.

### Composition with `.mdsmith.yml`

A file-defined kind and an inline
`kinds.<name>:` block under the same name
is a config error naming both sources. The
two sources do **not** merge — a merged
kind would defeat the "read one file to
know one kind" property this plan ships.

Two kind files with the same basename
across `*.yaml` and `*.yml` is also a
config error naming both files.

### Kind-assignment, overrides, ignore

These three sections stay in `.mdsmith.yml`.
They are glob-keyed and have no canonical
name. A kind-assignment entry may reference
either a file-defined or an inline kind by
name — the resolution map merges before
assignment runs.

### Schema sources within a kind file

A kind body accepts three schema sources:
an inline `schema:` map, a
`rules.required-structure.schema:` path to
a `proto.md`, and a legacy
`…inline-schema:` map. Setting two on the
same kind errors, caught pairwise by
`validateKindSchemaSources` in
[validate.go](../internal/config/validate.go).
The check applies unchanged to file kinds.

### Schema sharing across kinds

A schema shared across kinds uses
`extends:` (plan 135). A file-defined kind
may extend an inline kind and vice versa.
The existing cycle detection in
[kind_extends.go](../internal/config/kind_extends.go)
runs across both sources.

A path-shared `proto.md` schema works too.
A kind file may set
`rules.required-structure.schema: <path>`
to point at a workspace `proto.md`.

### CLI surface

`mdsmith kinds resolve <file>` prints the
defining-source path for each kind it
reports (the file under `.mdsmith/kinds/`
or `.mdsmith.yml`). No new subcommand.

### Out of scope but seeded

`.mdsmith/conventions/<name>.yaml` fits the
same shape. A later plan slots it into the
tree. The plan 113 convention body maps
cleanly to one file per name.

`overrides:`, `kind-assignment:`, and
`ignore:` are glob-keyed with no canonical
name and stay inline. The top-level
`rules:` block is a single project-wide
setting and stays inline too.

## Tasks

Per
[Go architecture patterns](../docs/development/architecture/go.md):
no new package is required. Discovery and
kind parsing live in `internal/config`;
schema parsing lives in `internal/schema`.

1. **`internal/config`**: add
   `discoverKinds(workspaceDir string)
   (map[string]discoveredKind, error)`.
   Walks `.mdsmith/kinds/*.{yaml,yml}` at
   the workspace root. Validates
   basenames, rejects subdirectories,
   rejects basename collisions across
   `*.yaml` and `*.yml`, rejects bodies
   with keys outside `KindBody`. Unit test
   `TestDiscoverKinds` with subtests per
   rejection case.
2. **`internal/config`**: update `Load` in
   [load.go](../internal/config/load.go) to
   call `discoverKinds` and merge the
   result into `cfg.Kinds`. A name
   colliding between a file kind and an
   inline kind is a config error naming
   both sources. Unit test
   `TestLoad_KindFileInlineCollision`.
3. **`internal/config`**: wire file-defined
   kinds through the existing
   `validateKindSchemaSources` in
   [validate.go](../internal/config/validate.go)
   so all three pairwise checks (`schema:`,
   `rules.required-structure.schema:`,
   `rules.required-structure.inline-schema:`)
   fire on a kind file the same way they
   fire on an inline kind. Pinned today by
   `TestKindRejectsDualSchemaSources`,
   `TestKindRejectsInlineMapInRules`, and
   `TestKindRejectsBothSchemaAndInlineUnderRules`
   in
   [schema_kinds_test.go](../internal/config/schema_kinds_test.go);
   add file-defined parallels of each.
4. **`internal/engine`**: no change. The
   resolved `cfg.Kinds` already drives
   schema resolution and rule layering.
5. **Provenance**: extend
   [provenance.go](../internal/config/provenance.go)
   so each kind layer records its
   defining-source path. Unit test
   `TestProvenance_KindSourcePath`.
6. **Contract test**: add
   `internal/integration/kind_file_contract_test.go`.
   Locks the directory layout, the
   basename rule, the dual-source
   rejection, the subdirectory rejection,
   and the no-extra-top-level-keys rule.
   Per
   [cross-system contracts](../docs/development/architecture/cross-system.md),
   every public surface ships with a
   contract test.
7. **Integration test**: parallel fixtures
   that share a Markdown body and assert
   a file-defined kind produces the same
   diagnostics as the equivalent inline
   kind. Lives under
   `internal/integration/`.
8. **CLI**: extend `mdsmith kinds resolve`
   to print the defining-source path next
   to each kind it reports.
9. **Docs**: add
   `docs/reference/kind-files.md`. Add a
   row to the boundaries table in
   [cross-system.md](../docs/development/architecture/cross-system.md)
   pointing at it. Extend
   [file-kinds.md](../docs/guides/file-kinds.md)
   with a "split a kind into its own file"
   recipe.
10. **Repo migration**: deferred. The
    pinned `mdsmith` version this repo
    lints itself with does not yet
    support `.mdsmith/kinds/`. Migration
    is scheduled for after the next
    release bumps the pinned version.
11. **Follow-up plan**: open a plan file
    (not implemented here) for
    `.mdsmith/conventions/<name>.yaml` so
    the directory convention is recorded.

## Acceptance Criteria

- [x] A kind at `.mdsmith/kinds/foo.yaml`
      with the same body as inline
      `kinds.foo:` emits byte-equal
      diagnostics. (LSP: substitutable.)
- [x] A kind declared both in a file and
      inline errors naming both sources.
- [x] Two kind files with the same
      basename across `.yaml`/`.yml` error
      naming both.
- [x] A basename failing `[a-z][a-z0-9-]*`
      or a file in a subdir of
      `.mdsmith/kinds/` errors; inline kind
      names stay unvalidated.
- [x] A kind file with a key outside
      `KindBody` errors naming key and
      file.
- [x] The three pairwise schema-source
      checks in `validateKindSchemaSources`
      fire on a file-defined kind the same
      way they fire on inline.
- [x] A file-defined kind may `extends:` an
      inline kind and the reverse; a cycle
      is detected.
- [x] A `kind-assignment:` entry resolves
      a file-defined kind by name with no
      extra wiring.
- [x] `mdsmith kinds resolve <file>` prints
      the defining-source path per kind.
- [x] `cross-system.md` boundaries table
      lists `.mdsmith/kinds/` with
      `docs/reference/kind-files.md`.
- [x] Every new function in
      `internal/config` ships with its
      dedicated unit test. (Test pyramid.)
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` clean.
- [x] `mdsmith check .` passes.
