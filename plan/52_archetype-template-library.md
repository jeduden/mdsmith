---
id: 52
title: Archetype / Template Library for Agentic Patterns
status: "✅"
---
# Archetype / Template Library for Agentic Patterns

## Goal

Let users define their own required-structure
archetype templates. Support common agentic Markdown
patterns. Expose first-class CLI verbs to init, list,
show, and locate archetypes.

mdsmith ships no built-in archetypes. Users point
mdsmith at archetype directories via config. The CLI
scaffolds, discovers, and inspects them. The
`required-structure` rule resolves `archetype: <name>`
against those roots.

## Tasks

1. Remove the embedded built-in archetype templates
   added in the first pass; delete
   `internal/archetypes/*.md` shipped assets and the
   `go:embed` registry that loads them.
2. Add archetype discovery config to `.mdsmith.yml`:
   `archetypes.roots` — ordered list of base
   directories searched for `<name>.md` schemas.
   First match wins; earlier roots shadow later ones.
3. Update `required-structure` rule so `archetype:
   <name>` resolves via the configured roots. Missing
   archetype returns a clear diagnostic listing the
   roots searched and the archetypes discovered.
4. Add `mdsmith archetypes init [dir]` CLI to
   scaffold a default archetypes directory (default:
   `./archetypes/`) containing a documented example
   schema. The command creates the directory if
   absent, writes the example only when it does not
   already exist, and prints follow-up config to add
   the root to `.mdsmith.yml` (it does not mutate the
   config file).
5. Add `mdsmith archetypes list` to print each
   discovered archetype as `<name>\t<path>`, one per
   line, sorted by name. Exit non-zero if no roots
   are configured.
6. Add `mdsmith archetypes show <name>` to print the
   archetype source (including front matter) to
   stdout. Exit non-zero with a clear error when the
   name does not resolve.
7. Add `mdsmith archetypes path <name>` to print the
   resolved filesystem path. Exit non-zero with a
   clear error when the name does not resolve.
8. Document archetype authoring, `archetypes.roots`
   configuration, and the new CLI verbs in
   `docs/guides/directives/enforcing-structure.md`
   and the MDS020 README. Include a short cookbook
   entry showing `init` → edit → apply via override.

## Out of Scope

- Shipping opinionated archetype templates. A
  separate, optional, `mdsmith archetypes init
  --with-examples` flag may be added later to copy
  community examples from a documented repo, but it
  is not part of this plan.
- Editing the user's `.mdsmith.yml` automatically;
  `init` only scaffolds the directory and prints the
  config snippet to add.

## Acceptance Criteria

- [x] No archetype templates are embedded in the
  binary; the linker-visible symbol footprint for
  archetypes contains no schema text.
- [x] `.mdsmith.yml` accepts `archetypes.roots` as a
  list of relative directories; the linter uses it
  to resolve `archetype: <name>` in required-structure
  settings.
- [x] `mdsmith archetypes init` creates the target
  directory with an example schema, refuses to
  overwrite an existing example, and prints a
  config snippet for the user to add.
- [x] `mdsmith archetypes list` prints all
  discovered archetypes, each with its source path,
  sorted by name.
- [x] `mdsmith archetypes show <name>` prints the
  raw schema source.
- [x] `mdsmith archetypes path <name>` prints the
  filesystem path.
- [x] Unknown archetype names produce an error
  that names the configured roots and lists nearby
  candidates.
- [x] Guide + MDS020 README document authoring,
  `archetypes.roots` configuration, and each CLI
  verb.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
