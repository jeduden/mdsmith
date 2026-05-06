---
id: 132
title: Inline schema in the `kinds` map
status: "🔲"
model: opus
summary: >-
  Let `kinds:` carry a `schema:` block directly so a
  small schema (front matter constraints, heading
  template, filename glob) does not require a
  separate `proto.md`. The schema engine receives
  the same parsed AST whether the source is inline
  YAML or a referenced file.
---
# Inline schema in the `kinds` map

## Goal

Let a `kinds:` entry declare its schema inline.
The schema sits alongside the rule overrides.
A small schema — a few front-matter fields, a
flat heading template, a filename glob — should
fit in the config the user already maintains.
No separate `proto.md` for the small case.

## Background

Today every kind that wants front-matter or
structure validation sets
`required-structure.schema:` to a path. The schema
file is a Markdown document whose front matter
holds CUE patterns and whose body is the heading
template. That path makes sense for plans
([`plan/proto.md`](../plan/proto.md)) and rule
docs, where the body is a real heading template
worth reading on its own. It is overhead for kinds
whose schema is "two FM fields and a filename
glob".

The
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md)
records this gap as **S-1**. mdbase keeps its
typing in a single config file
(`_types/<name>.md`); mdsmith asks for one file
per kind. The cost is small per kind and high in
aggregate when a project has eight or ten kinds.

## Design

### Config shape

A kind accepts a `schema:` block alongside
`rules:`:

```yaml
kinds:
  task:
    rules:
      line-length:
        max: 100
    schema:
      frontmatter:
        id: '=~"^TASK-[0-9]{4}$"'
        status: '"open" | "in-progress" | "done"'
        priority: 'int & >=1 & <=5'
        "due?": "string"
      structure:
        - "# {title}"
        - "## Goal"
        - "## Acceptance"
      require:
        filename: "TASK-[0-9]+.md"
```

Three sub-blocks, each optional:

- `frontmatter:` — a map from field name to a CUE
  expression (string). Keys ending in `?` are
  optional, matching CUE's existing convention.
- `structure:` — a list of heading template lines.
  Each line is the same Markdown a `proto.md` body
  would contain.
- `require:` — the same fields the `<?require?>`
  directive accepts today (`filename:` to start).

The schema loader reads the block, synthesizes the
equivalent `proto.md` AST in memory, and hands it
to the existing schema engine. The engine does
not learn a second representation.

### Coexistence with file schemas

`required-structure.schema:` continues to work for
kinds whose schema is large enough to want a real
file. A kind can set **either** `schema:` (inline)
**or** `rules.required-structure.schema:` (path),
not both — the loader rejects a kind that sets
both with a config error naming the kind and both
sources.

### Surface

Three things change for the user:

- New `schema:` key in each kind.
- `mdsmith kinds` (plan 95) shows whether a
  kind's schema is `inline` or `file:<path>` so a
  user can tell at a glance which kinds carry
  config-embedded constraints.
- `required-structure.schema:` keeps its current
  behavior. Existing repos see no change unless
  they opt in.

### Composition with directives

`structure:` lines may use `<?include?>` for
fragment composition the same way a `proto.md`
body does. This keeps inline and file schemas at
parity: a project that starts inline and grows
out can move the block to a file without changing
its semantics.

## Non-Goals

- New schema features. The expressiveness of
  `frontmatter:` and `structure:` exactly matches
  what `proto.md` supports today.
- A migration tool that converts `proto.md` files
  into inline blocks (or vice versa). Manual
  rewrite is straightforward; automation pays off
  only if a real project needs it.

## Tasks

1. Extend the kind config struct in
   `internal/config/` with an optional
   `Schema *KindSchema` field carrying
   `Frontmatter`, `Structure`, and `Require`
   sub-blocks.
2. Add a synthesizer in
   `internal/rules/requiredstructure/` that turns
   a `KindSchema` into the in-memory AST the rule
   already consumes from a parsed `proto.md`.
3. Reject configs that set both `schema:` (inline)
   and `rules.required-structure.schema:` (path)
   on the same kind. The error names the kind and
   both sources.
4. Teach the loader to feed inline schemas through
   the same caching path file schemas use, keyed
   on the kind name plus a content hash.
5. Update `mdsmith kinds` output (plan 95) to
   show `schema: inline` or `schema: file:<path>`
   per kind.
6. Document inline schemas in
   [`docs/guides/file-kinds.md`](../docs/guides/file-kinds.md)
   with a worked task example. Cross-link from
   the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md).
7. Add a fixture under
   `internal/rules/MDS020-required-structure/good/`
   exercising an inline-schema kind, and a `bad/`
   counterpart whose front matter violates the
   inline `frontmatter:` constraints.
8. Unit tests:

  - inline `frontmatter:` produces the same
     diagnostics a path-equivalent `proto.md`
     would,
  - inline `structure:` validates heading
     sequences identically,
  - inline `require.filename:` validates basenames
     identically,
  - dual-source error fires with both schemas set.

## Acceptance Criteria

- [ ] A kind with `schema.frontmatter:` validates
      a file's front matter and produces the same
      MDS020 diagnostics as the equivalent
      `proto.md`-referenced kind (regression test
      compares both).
- [ ] A kind with `schema.structure:` validates
      heading sequences identically to a
      file-schema kind.
- [ ] A kind with `schema.require.filename:`
      validates basenames identically to a
      `<?require filename:?>` directive in a file
      schema.
- [ ] A kind that sets both `schema:` and
      `rules.required-structure.schema:` produces
      a config error naming the kind and both
      sources.
- [ ] `mdsmith kinds` reports `schema: inline` for
      inline-schema kinds and `schema:
      file:<path>` for file-schema kinds.
- [ ] [docs/guides/file-kinds.md](../docs/guides/file-kinds.md)
      describes the inline form with one worked
      example.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
