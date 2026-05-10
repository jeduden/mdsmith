---
id: 140
title: "Per-kind `path-pattern` for filename validation"
status: "🔲"
model: sonnet
depends-on: [147]
summary: >-
  Let a kind declare a `path-pattern:` glob that the
  file's path must match. Replaces hand-rolled CI
  scripts that enforce filename conventions and
  produces an actionable MDS020 diagnostic when a
  file claims a kind whose path it does not fit.
---
# Per-kind `path-pattern` for filename validation

## Goal

Let a kind declare a path-shape constraint
alongside its rule overrides and schema. A file
with `kinds: [plan]` whose path does not match
the kind's `path-pattern:` glob — e.g. one
under `plan/` not starting with digits when the
project requires `plan/[0-9][0-9]*_*.md` —
produces a clear diagnostic without a custom
CI script. The constraint is a glob; full
"integer-only" enforcement is not expressible
(globs cannot say "exactly digits, no other
characters" without listing each digit
position), so the example pattern is an
approximation, not a regex.

## Background

Plan **M-3** in the
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md).
mdbase ships `path_pattern:` as a kind property
that both validates existing files and templates
new ones. mdsmith has the validation half today
only via the per-schema `<?require filename:?>`
directive — invisible from the kind config and
limited to the schema-file path.

The trigger is a project enforcing filename
conventions (RFC numbering, plan IDs, runbook
slugs) via custom CI. Lifting the pattern into
the kind config replaces the script and makes the
constraint visible in `mdsmith kinds`.

## Non-Goals

- Templating new files. Patterns like `{id:int}`
  hint at scaffolding (C-1 / `mdsmith create`),
  but this plan ships validation only. The
  pattern is a glob; integers are handled as
  glob character classes for now.
- Replacing `<?require filename:?>`. The
  directive stays as the file-schema surface;
  `path-pattern:` is the kind-config surface.
  Plan 146 wires both onto the same engine, so
  diagnostics are consistent.
- Multiple patterns per kind. One pattern per
  kind keeps the surface small. A kind with two
  layouts can be split into two kinds.

## Design

### Config shape

```yaml
kinds:
  plan:
    path-pattern: "plan/[0-9][0-9]*_*.md"
    schema:
      frontmatter:
        id: "int & >=1"

  rfc:
    path-pattern: "docs/rfc/RFC-[0-9][0-9][0-9][0-9].md"
```

The pattern is a glob in the same syntax the
[globs reference](../docs/reference/globs.md)
documents. It anchors on the workspace root.

A kind without `path-pattern:` keeps its current
behavior — no filename constraint.

### Validation

Take a file whose effective kind has a
`path-pattern:`. The workspace-relative path
must match the glob. On a mismatch, mdsmith
emits an MDS020 diagnostic through plan 147's
actionable shape:

```text
filename: got "plan/early-draft.md", expected
  glob plan/[0-9][0-9]*_*.md
schema: kinds[plan] / path-pattern
```

The diagnostic is anchored at line 1 of the
file (front matter is the closest "where the
constraint applies" anchor; the file's body
has nothing to point at).

### Composition with `<?require filename:?>`

Both surfaces feed the same matcher. When a
kind has `path-pattern:` and its schema also
declares `<?require filename:?>`, both must
match. The diagnostic names whichever fails
first in evaluation order: `path-pattern:`
first (it is the kind-level constraint), then
the schema directive.

If both fail, both diagnostics fire. The user
sees what the schema requires *and* what the
kind requires.

### Surfacing

`mdsmith kinds show <name>` prints the pattern
when set:

```text
plan:
  schema: file:plan/proto.md
  path-pattern: plan/[0-9][0-9]*_*.md
```

A reader can audit constraints from one
command.

## Tasks

1. Extend the kind config struct in
   `internal/config/` with an optional
   `PathPattern string` field.
2. Wire validation into the same path the
   `<?require filename:?>` directive uses
   (likely a shared helper in
   `internal/rules/requiredstructure/`).
3. Emit the diagnostic via the actionable
   shape from plan 147, with `field:
   filename`, `actual:` the workspace path,
   `expected:` "glob `<pattern>`",
   `schema_ref: kinds[<name>] / path-pattern`.
4. When both `path-pattern:` and a schema
   `<?require filename:?>` are present, run
   both and emit one diagnostic per failure.
5. Extend `mdsmith kinds show <name>` output
   to include `path-pattern:` when set.
6. Update
   [`docs/guides/file-kinds.md`](../docs/guides/file-kinds.md)
   with a worked example.
7. Tests:

  - a file matching the pattern produces no
    diagnostic,
  - a file claiming the kind whose path does
    not match produces a clear diagnostic,
  - the diagnostic uses plan 147's shape,
  - `path-pattern:` plus
    `<?require filename:?>` together emit
    one diagnostic per failure,
  - kinds without `path-pattern:` retain
    current behavior (regression).

## Acceptance Criteria

- [ ] A kind with `path-pattern:` validates
      the workspace-relative path of every
      file assigned to that kind.
- [ ] A path mismatch produces an MDS020
      diagnostic in the plan 147 shape with
      `field: filename`.
- [ ] A kind with both `path-pattern:` and a
      schema `<?require filename:?>` emits
      one diagnostic per failing constraint.
- [ ] Kinds without `path-pattern:` retain
      current behavior (regression test).
- [ ] `mdsmith kinds show <name>` shows
      `path-pattern:` when set.
- [ ] [`docs/guides/file-kinds.md`](../docs/guides/file-kinds.md)
      documents the new field with one worked
      example.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
