---
id: 136
title: Field deprecation flag in schemas
status: "🔲"
model: sonnet
depends-on: [146, 147]
summary: >-
  Let a schema mark a front-matter field
  deprecated. MDS020 emits a warning (not an
  error) when the field appears, naming the
  replacement and the schema that introduced the
  deprecation. Soft-deprecation lets a project
  migrate without breaking the build.
---
# Field deprecation flag in schemas

## Goal

Let a schema declare a field deprecated and let
mdsmith warn — but not fail — when files still
carry it. The warning names the replacement
field and the schema where the deprecation
lives, so the user can fix the file without
opening the schema.

## Background

The mdbase research records this as **S-6**.
Schema migration is a real workflow: a project
renames `legacy_owner` to `owner`, adds the new
field, and wants to clean up the old one over
weeks or months without breaking CI.

Today MDS020 has only two postures: a field is
in the schema (validated) or it is not (allowed
or forbidden depending on whether the schema is
closed). There is no middle ground for "tolerate
but warn".

## Non-Goals

- Auto-rewrite. A `mdsmith fix` that copies
  `legacy_owner` into `owner` is plausible, but
  the field-rename mapping is project-specific
  and out of scope here. This plan only emits
  the warning.
- A separate rule. The deprecation diagnostic
  is an MDS020 sub-diagnostic with a different
  severity, not a new rule code.
- Deprecating sections (`structure:` entries).
  Sections come and go via `<?allow-empty-section?>`
  already; no real workflow is asking for soft
  deprecation of structure.

## Design

### Schema-side declaration

A field's value can be a map carrying metadata
instead of the bare CUE expression:

```yaml
schema:
  frontmatter:
    legacy_owner:
      type: string
      deprecated: true
      message: 'use "owner" instead'
    owner:
      type: string
```

The loader recognizes the map form when it sees
a `type:` key (which is otherwise illegal at the
top of a CUE expression in this position). The
short string form continues to work for the
common case.

### File-schema declaration

A `proto.md` schema uses the same shape inside
its CUE front matter. CUE structs support
extra metadata fields out of the box; the
loader treats `deprecated`, `message`, and
`replaced-by` as known keys and the rest as
constraints.

### Diagnostic

When a file's front matter contains a
deprecated field:

```text
legacy_owner: deprecated field
  message: use "owner" instead
  schema: plan/proto.md:12
severity: warning
```

The diagnostic uses plan 147's shape with
`Severity: Warning` instead of `Error`. The
file's exit code (with `--error-on warning`
unset) does not change.

### Replacement hint

`replaced-by:` is a richer alternative to
`message:` — when set, mdsmith can render a
canonical sentence:

```yaml
legacy_owner:
  type: string
  deprecated: true
  replaced-by: owner
```

→

```text
legacy_owner: deprecated; replaced by `owner`
```

A schema may set both; `message:` wins for the
human-facing line, `replaced-by:` is exposed in
the structured diagnostic for tooling.

### Deprecating without removing

A deprecated field stays in the schema and
continues to be validated. A file with
`legacy_owner: 42` (where the schema says
`legacy_owner.type: string`) emits two
diagnostics: a warning for deprecation and an
error for the type violation. The deprecation
does not mute other checks.

### Removing the field

When the project is ready to remove the
deprecated field entirely, the maintainer
deletes the entry from the schema. From that
point the field is "unknown" and falls under the
schema's existing closed/open posture.

## Tasks

1. Extend the inline-schema parser (plan 146)
   and the `proto.md` front-matter parser to
   accept the map form (`{type, deprecated,
   message, replaced-by}`) for a field.
2. Pipe the deprecation metadata into the
   schema engine's per-field state.
3. Emit a Warning-severity diagnostic when a
   deprecated field is present in front matter,
   using plan 147's `SchemaDiagnostic` shape.
4. Continue evaluating the field's `type:`
   constraint; emit a separate diagnostic if it
   fails. The warning and the type error
   coexist.
5. Add a `Deprecated bool` and `ReplacedBy
   string` to the structured diagnostic so LSP
   clients and CI scripts can route warnings
   without parsing the message.
6. Document deprecation semantics in the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md)
   and link from the
   [file-kinds guide](../docs/guides/file-kinds.md).
7. Tests:

  - a deprecated field present in FM emits a
     Warning diagnostic with the `message:`
     payload,
  - `replaced-by:` renders the canonical
     sentence in the absence of `message:`,
  - a deprecated field that violates its type
     emits both a deprecation Warning and a
     type Error,
  - removing the field from the schema returns
     the file to its closed/open posture.

## Acceptance Criteria

- [ ] An inline-schema field with `deprecated:
      true` emits a Warning-severity MDS020
      diagnostic when present in a file's FM.
- [ ] A file-schema field with `deprecated:
      true` does the same.
- [ ] `message:` text appears in the
      diagnostic; `replaced-by:` renders a
      canonical sentence when `message:` is
      absent.
- [ ] A deprecated field that also violates its
      `type:` produces two diagnostics — one
      Warning, one Error.
- [ ] Exit code is unchanged when only Warning
      diagnostics fire (default `--error-on`
      threshold).
- [ ] Removing the field from the schema
      returns the file to its prior posture
      (no warning, closed/open behavior
      unchanged).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
