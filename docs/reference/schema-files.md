---
title: Schema files under `.mdsmith/schemas/`
weight: 27
summary: >-
  Each file under `.mdsmith/schemas/` declares one
  named schema. The basename is the schema name; the
  body carries the inline `schemas.<name>:` keys. A
  kind references one by name (`schema: rfc-v1`).
---
# Schema files under `.mdsmith/schemas/`

A **schema file** is a YAML file under
`.mdsmith/schemas/` whose basename is the schema's
name and whose body is a document-structure schema.
One file per schema, no nesting. The directory sits
next to `.mdsmith.yml` at the workspace root.

```text
.mdsmith.yml                   # unchanged
.mdsmith/
  kinds/
    audit-log.yaml
  conventions/
    portable-strict.yaml
  schemas/
    rfc-v1.yaml
    runbook.yaml
```

A schema file is the per-file split of an inline
`schemas.<name>:` registry entry. A kind references
the schema by name (`schema: rfc-v1`); one schema
file can drive several kinds. Use schema files when
a schema is shared across kinds, or when an inline
`schema:` block has grown large enough to crowd the
kind it sits on.

## File shape

The file body matches the body of an inline
`schemas.<name>:` registry entry. It parses through
the same matcher engine an inline `schema:` block on
a kind uses. The allowed top-level keys are:

- `frontmatter`
- `filename`
- `closed`
- `sections`
- `cross-references`
- `acronyms`
- `index`

A key outside that set is a config error naming the
key and the file. The [schemas guide](../guides/schemas.md)
documents what each key constrains.

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
  - heading:
      regex: '.+'
      repeat: { min: 0 }
  - heading: "References"
```

## Basename rule

The schema's name is the basename minus extension.
The basename must match `[a-z][a-z0-9-]*` — lower
case, starting with a letter, with optional
hyphen-separated segments. The same pattern governs
a named `schema:` reference on a kind, so a
`schema: rfc-v1` value and a
`.mdsmith/schemas/rfc-v1.yaml` file name the same
entry. The rule applies only to filenames; inline
`schemas.<name>:` keys stay unvalidated.

Both `*.yaml` and `*.yml` are scanned. Two schema
files with the same basename across the two
extensions is a config error naming both files.

Subdirectories under `.mdsmith/schemas/` are
rejected, as are symlinks. A file larger than 1 MB
is rejected. A file with no parseable body is
rejected. One schema per file, flat layout.

## Composition with `.mdsmith.yml`

A top-level `schemas:` block inside `.mdsmith.yml`
holds the same named schemas inline. A project can
mix inline-registry and file-defined schemas freely.

The same schema name declared in **both** a file and
inline under `schemas:` is a config error naming both
sources. The two sources do **not** merge — a merged
schema would defeat the "read one file to know one
schema" property schema files ship.

A kind references a schema by name with the
polymorphic `schema:` key:

```yaml
# .mdsmith.yml
schemas:                  # inline equivalent of
  draft:                  # .mdsmith/schemas/draft.yaml
    filename: "DRAFT-*.md"

kinds:
  rfc:
    schema: rfc-v1        # references .mdsmith/schemas/rfc-v1.yaml
  rfc-internal:
    schema: rfc-v1        # one schema drives both kinds
  proposal:
    schema: draft         # references the inline registry entry
  note:
    schema:               # an inline body is still allowed
      filename: "NOTE-*.md"
```

A `schema:` scalar is a registry reference; a
`schema:` mapping is an inline body carried on the
kind. The resolver replaces a named reference with
the schema's body before the kind validates, so a
referencing kind behaves exactly as a kind with the
same body inline. An undeclared name is a config
error naming the kind and the missing schema.

A named `schema:` is mutually exclusive with the
other two schema sources on the same kind — a
`rules.required-structure.schema:` path and a
`rules.required-structure.inline-schema:` map. Two
sources on one kind is a config error quoting "pick
one source". A kind's top-level `path-pattern:` runs
independently of the resolved schema's `filename:`;
both fire on a mismatch.

`extends:`, `kind-assignment:`, `overrides:`, and
`ignore:` stay in `.mdsmith.yml` or in kind files.
Registry references resolve to bodies before the
`extends:` chain merges, so a schema file composes
across an inheritance chain the same way an inline
schema does.

## Audit

`mdsmith kinds resolve <file>` prints the schema's
defining-source path next to each kind whose schema
comes from a separate file:

```text
file: docs/rfcs/RFC-0001.md
effective kinds:
  - rfc (from kind-assignment[0]: glob docs/rfcs/*.md) defined-in .mdsmith.yml schema-in .mdsmith/schemas/rfc-v1.yaml
```

The `defined-in` clause names the file that declared
the kind; `schema-in` names the file that declared
its schema. They differ when a kind in `.mdsmith.yml`
references a schema under `.mdsmith/schemas/`, so you
can jump straight to the schema rather than the
referencing kind.

The JSON shape (`--json`) carries a
`schema-source-path:` key on each resolved-kind
entry. It is set three ways: a named-YAML reference
reports the `.yaml` path, an inline-registry
reference reports `.mdsmith.yml`, and a `proto.md`
schema reports the referenced path. It is omitted for
an inline-on-kind schema. That schema's
`source-path:` already names the file.

## See also

- [Schemas](../guides/schemas.md) — the three
  schema sources and the matcher grammar.
- [Kind files](kind-files.md) — one file per kind,
  the parallel directory.
- [Convention files](convention-files.md) — one
  file per user convention, the parallel directory.
- [Section schema](section-schema.md) — the
  entry-shape grammar in full.
