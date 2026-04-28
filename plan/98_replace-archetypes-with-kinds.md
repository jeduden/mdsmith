---
id: 98
title: Replace `archetypes` with `kinds`
status: "🔳"
model: sonnet
summary: >-
  Remove the `archetypes` CLI subcommand, config key,
  and doc directory. The `kinds` model from plans
  92/95 is the single, generalized concept. No
  backward-compat shim.
---
# Replace `archetypes` with `kinds`

## Goal

Today mdsmith has two overlapping concepts:
`archetypes` for named schema files, and `kinds` for
rule-config bundles. This plan collapses them into
one. The following surface goes away:

- the `archetypes` CLI subcommand,
- the `archetypes.roots:` config key,
- the `internal/archetypes` package,
- the `docs/background/archetypes/` doc directory.

Kinds take over the schema-by-name role through
paths declared in their bodies.

No backward-compat shim is shipped — the user has
opted to break in place. Existing configs that use
`archetypes.roots:` or `archetype:` lookups must
migrate to `kinds:` with explicit `schema:` paths.

## Background

Today mdsmith has two parallel concepts of "named
schema":

1. `archetypes` — the `archetypes` CLI subcommand
   (`init` / `list` / `show` / `path`), the
   `archetypes.roots:` config key, the
   `internal/archetypes` Resolver, and a doc
   directory `docs/background/archetypes/`.
   Documented at
   [internal/archetypes/archetypes.go](../internal/archetypes/archetypes.go):
   *"An archetype is a Markdown schema file whose
   basename (without the ".md" extension) is the
   archetype name."*
2. `kinds` (plans 92/95) — a named bundle of rule
   settings that may include
   `rules.required-structure.schema:` pointing to a
   schema file by path.

The two collide:

- Internally on the term: `archetypes` (CLI/config:
  schema files) vs `archetypes` (docs convention:
  rule patterns).
- Externally with Hugo, where `archetype` means a
  content scaffold — different from both.
- Functionally: a kind with `required-structure
  .schema:` is a generalization of an archetype.

## Design

### Removed surface

- CLI: `mdsmith archetypes init/list/show/path`.
- Config key: `archetypes:` (with `roots:` field).
- Go package: `internal/archetypes/` and its
  `Resolver`.
- Doc directory: `docs/background/archetypes/`. Its
  one current page (`generated-section`) moves to
  `docs/background/concepts/generated-section.md`.

### Equivalent functionality under `kinds`

- `archetypes list` → `mdsmith kinds list`
  (introduced in plan 95): lists declared kinds with
  their merged bodies. Each kind that sets
  `rules.required-structure.schema:` is the new
  "named schema".
- `archetypes show <name>` → `mdsmith kinds show
  <name>`: prints one kind's body.
- `archetypes path <name>` → `mdsmith kinds path
  <name>`: prints the kind's `schema:` path.
- `archetypes init [dir]` → drop. Users hand-write
  `kinds:` blocks; `mdsmith init` covers fresh-repo
  scaffolding.

### Schema resolution by name

The `archetypes.roots:` mechanism let `required-
structure` accept a name (`schema: story`) and look
it up across configured roots. This is removed.
Kinds declare `schema:` as an explicit path. Any
project that relied on name lookup migrates to
explicit paths in kind bodies.

### Doc directory rename

The `archetypes/` doc directory has one page today,
`generated-section`. The page moves under
`docs/background/concepts/`, alongside plan 93's
placeholder-grammar page.

## Tasks

1. Remove `cmd/mdsmith/archetypes.go`, the
   `archetypes` dispatch in `main.go`, and the
   `internal/archetypes/` package. Update affected
   tests.
2. Remove the `archetypes:` config key (and `Roots`
   field on `Config`). Remove its loader code and
   the `ValidateRoots` helper.
3. Remove the name-lookup path in `required-
   structure`'s schema resolution. The rule's
   `schema:` setting accepts only a path now.
4. Move
   `docs/background/archetypes/generated-section/README.md`
   to `docs/background/concepts/generated-section.md`.
   Update internal links throughout the repo.
5. Move the placeholder-grammar concept page (plan
   93) to `docs/background/concepts/` if it is not
   already there.
6. Delete `docs/background/archetypes/` (including
   its README) once empty.
7. Update `CLAUDE.md`'s catalog directive include
   list to drop the archetypes glob and add the
   concepts glob.
8. Update `docs/reference/cli.md` to remove the
   `archetypes` subcommand section. The `kinds`
   subcommand replacement is documented under plan
   95.
9. Update `mdsmith init`: the generated
   `.mdsmith.yml` must contain no `archetypes:` key
   and must remain accepted by the loader. Add a
   regression test that runs `init` in a tempdir and
   greps for the absence of `archetypes`.

## Acceptance Criteria

- [ ] `mdsmith archetypes` exits 2 with "unknown
      command".
- [ ] `mdsmith kinds list` is the only listing
      surface for named schemas.
- [ ] `archetypes:` keys in `.mdsmith.yml` produce a
      config error directing the user to `kinds:`.
- [ ] `required-structure.schema:` accepts a path;
      a name (e.g. `schema: story`) produces a clear
      error.
- [ ] `docs/background/concepts/generated-section.md`
      exists; `docs/background/archetypes/` no
      longer exists.
- [ ] `mdsmith check .` is green after all renames
      (internal links updated).
- [ ] `internal/archetypes/` no longer exists; no
      package imports it.
- [ ] `mdsmith init` in a fresh directory writes a
      `.mdsmith.yml` containing no `archetypes:` key;
      the file is accepted by the loader (covered by
      test).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
