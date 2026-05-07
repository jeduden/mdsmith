---
id: 132
title: Schema engine — sources, scope tree, per-scope rules
status: "🔲"
model: opus
summary: >-
  Replace MDS020's heading-template engine with
  an AST-rooted scope tree. Schemas come from two
  sources (inline `kinds.<name>.schema:` or a
  `proto.md` file) and parse to one in-memory
  representation. Each scope binds an AST subtree
  to per-rule config overrides; existing rules
  reuse-with-no-code-change. Foundation for plans
  142 (content constraints) and 143
  (cross-references, acronyms, index).
---
# Schema engine — sources, scope tree, per-scope rules

## Goal

Promote MDS020's heading-template engine into a
small schema engine with two sources and one
scope-tree representation. Make per-section
rule config the primary way to express
"this section is stricter than the rest of the
document".

The bigger schema directions — content rules,
cross-references, acronyms, index — sit on top
of this foundation in plans 142 and 143.

## Background

Today MDS020 reads a `proto.md` file. Its front
matter holds CUE constraints; its body is a
flat heading template plus optional
`<?require filename:?>`. Two limits show up:

- A small schema needs a separate file (gap
  **S-1**).
- A rule can be configured per file but not per
  section. A document with one strict section
  and a lenient body has no surface today.

This plan ships the inline-schema source and
the per-scope rule override. Plans 142 and 143
add the rich content / cross-ref / acronym /
index shapes.

## Non-Goals

- Content constraints, cross-references,
  acronyms, index. Plans 142 and 143.
- Auto-fix for new diagnostics. Auto-fix stays
  where rules already support it.
- Schema versioning. V-1.

## Design

### Two sources, one engine

- **Inline.** A `schema:` block on a kind in
  `.mdsmith.yml`.
- **File.** A `proto.md` referenced by
  `rules.required-structure.schema:`.

A kind sets at most one source. The loader
rejects a kind with both, naming the kind and
both paths. Both parse to one in-memory
`Schema` struct.

### Scope tree

A schema is a tree of scopes. A scope binds an
AST subtree to constraints (presence, aliases)
plus per-rule config overrides that apply only
inside that subtree. The root scope covers the
whole document; section scopes nest. Today's
flat heading template is the no-children case.

### Front matter and filename

```yaml
schema:
  frontmatter:
    id: '=~"^RFC-[0-9]{4}$"'
    status: '"draft" | "ratified" | "deprecated"'
    authors: '[...string] & len(authors) >= 1'
    created: date
  require:
    filename: "RFC-[0-9][0-9][0-9][0-9].md"
```

CUE per FM key. `?` for optional. Plan 134
(shortcuts), 135 (`extends:`), and 136
(deprecation) attach here.
`require.filename:` uses glob syntax.

### Section tree

```yaml
schema:
  sections:
    - heading: "## Overview"
      required: true
    - heading: "## Symptoms"
      required: true
      aliases: ["## Indicators"]
    - heading: "## Diagnosis"
      required: true
      children:
        pattern: "### Step {n}"
        sequential: true
        min: 1
        fields:
          - heading: "#### Check"
            required: true
          - heading: "#### Expected"
            required: true
    - heading: "## References"
      required: false
```

Section keys:

- `heading:` — literal heading; level from
  `#` count.
- `required:` — default `true`.
- `aliases:` — alternate headings.
- `children:` — recursive template with a
  `pattern:` for child headings. `{n}` is a
  sequence number, `{slug}` is any
  identifier. `sequential: true` enforces no
  gaps and no duplicates.
- `fields:` — nested sections inside each
  child match.

Order matters at each level. Out-of-order
sections produce a diagnostic naming the
expected and actual sequences.

### Per-scope rule overrides

Any scope may carry a `rules:` block:

```yaml
schema:
  sections:
    - heading: "## Decision"
      required: true
      rules:
        paragraph-readability:
          max-readability: 12.0
        max-section-length:
          max-words: 200
```

The override applies only inside that scope.
The merge stacks on top of the file's
effective config: defaults → kinds → file
globs → schema scope. Existing rules need no
changes — the engine threads the right config
through the subtree walk. Same
`paragraph-readability` runs document-wide and
section-scoped; only the config differs.

### Coexistence with existing rules

Existing rules read the same AST as the schema
engine. They accept per-section config through
the scope tree with no code change to any
`Configurable` rule. The engine emits
diagnostics through the same `lint.Diagnostic`
shape. The schema ships as wiring on top of the
existing rule set, not a parallel system.

## Tasks

1. Define `internal/schema/Schema`:
   `Frontmatter`, `Require`, and `Sections []Scope`.
   Each `Scope` carries `Heading`, `Required`,
   `Aliases`, `Children`, `Fields`, and
   `Rules`.
2. Build two parsers feeding the same struct:
   inline (YAML under `kinds.<name>.schema:`)
   and file (`proto.md` extended).
3. Reject configs that set both inline and
   file sources on one kind.
4. Re-implement MDS020 on top of the schema
   engine. Today's heading-template behavior
   becomes the flat-sections code path; every
   existing fixture passes unchanged.
5. Implement the section-tree validator:
   presence, aliases, child patterns,
   sequential numbering, recursion into
   `fields:`. Diagnostics use plan 133's shape.
6. Plumb per-scope rule-config overrides.
   While walking a section's subtree, apply
   the section's `rules:` overrides on top of
   the file's effective config.
7. Document the engine in the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md).
   Add a starter guide at
   `docs/guides/schemas.md` (subsections for
   plans 142 and 143 follow).
8. Add fixtures: a runbook exercising the
   section tree; a per-scope-rule fixture
   with the same prose in two sections,
   showing the scoped
   `paragraph-readability` override fires
   only in one.

## Acceptance Criteria

- [ ] An inline `schema:` block (front matter
      + flat sections) emits the same
      diagnostics as the equivalent
      `proto.md`-referenced kind.
- [ ] A schema with a section tree validates
      presence, aliases, child patterns, and
      sequential numbering on a runbook
      fixture.
- [ ] A schema `rules:` block on a section
      applies the override to that section
      only (verified with same prose in two
      sections).
- [ ] Setting both `schema:` and
      `rules.required-structure.schema:` on a
      kind produces a config error naming the
      kind and both sources.
- [ ] All existing MDS020 fixtures pass
      against the new engine without
      modification.
- [ ] The MDS020 README documents the engine
      with one worked example.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
