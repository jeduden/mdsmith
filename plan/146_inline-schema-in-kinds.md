---
id: 146
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

Promote MDS020's heading-template engine into
a small schema engine with two sources and one
scope-tree representation. Per-section rule
config becomes the way to say "this section is
stricter than the rest of the document". The
bigger schema directions — content rules,
cross-references, acronyms, index — sit on
top in plans 142 and 143.

## Background

MDS020 today: `proto.md` front matter holds CUE
constraints, body is a flat heading template
plus optional `<?require filename:?>`. Limits:
a small schema needs a separate file (gap
**S-1**), and rule config is per file, not per
section. This plan ships the inline source,
the recursive section tree, and the per-scope
rule override; plans 142 and 143 add content
rules, cross-refs, acronyms, and index. The
choice of language for FM and body is the
subject of an in-flight
[schema-unification spike](../docs/research/schema-unification/spike.md);
its recommendation folds back into this plan.

## Non-Goals

- Content rules, cross-refs, acronyms, index
  (plans 142 / 143).
- Auto-fix for new diagnostics.
- Schema versioning (V-1).

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

CUE per FM key. `?` for optional. Plan 148
(shortcuts), 135 (`extends:`), and 136
(deprecation) attach here.
`require.filename:` uses glob syntax.

### Section tree

`sections:` is one recursive list. The level of
each section is its depth in the tree: H2 at
the root's `sections:`, H3 inside that, and so
on. The document's H1 is reserved for the title
and is constrained separately (via the
`first-line-heading` rule and any `title:` FM
field).

```yaml
schema:
  sections:
    - heading: "Overview"
      required: true
    - heading: "Symptoms"
      required: true
      aliases: ["Indicators"]
    - heading: "Diagnosis"
      required: true
      sections:
        - heading: "Step {n}"
          repeats: true
          sequential: true
          min: 1
          sections:
            - heading: "Check"
              required: true
            - heading: "Expected"
              required: true
            - heading: "If different"
              required: false
    - heading: "References"
      required: false
```

Section keys:

- `heading:` — the heading text. No `#`
  markers; the level comes from depth.
  Placeholders allowed: `{n}` (sequence
  number), `{slug}` (any identifier),
  `{title}` (free text).
- `required:` — default `true`.
- `aliases:` — alternate heading texts.
- `sections:` — nested sections (one level
  deeper).
- `repeats:` — when `true`, the heading is a
  pattern; the document may have zero or more
  sections matching it.
- `sequential:` — on a repeating section,
  enforces no gaps and no duplicates in `{n}`.
- `min:` / `max:` — bounds on a repeating
  section's match count.

### Order, openness, unknown sections

A scope asserts two things: required sections
are present, and listed sections appear in the
declared order. Optional sections may be
skipped without breaking neighbors' order.

By default a scope is **open**: unlisted
headings are allowed anywhere among the listed
sections. `closed: true` makes the scope
strict; an unlisted heading then produces a
diagnostic.

```yaml
schema:
  closed: true
  sections:
    - heading: "Overview"
    - heading: "Decision"
```

`closed:` is per-scope. A strict root with
permissive subsections sets `closed: true` at
the root and omits it on each child.

A `"..."` entry is a positional escape hatch.
It does not require any heading. It tolerates
any unlisted sections at that position even
under `closed: true`:

```yaml
schema:
  closed: true
  sections:
    - heading: "Overview"
    - "..."
    - heading: "References"
```

The schema requires Overview first. References
last. Anything between. Nothing before
Overview or after References.

Out-of-order listed sections produce a
diagnostic naming expected and actual.

### Per-scope rule overrides

Any scope may carry a `rules:` block:

```yaml
schema:
  sections:
    - heading: "Decision"
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

Existing rules read the same AST. They accept
per-section config through the scope tree with
no code change to any `Configurable` rule. The
engine emits diagnostics through the existing
`lint.Diagnostic` shape. The schema is wiring,
not a parallel system.

## Tasks

1. Define `internal/schema/Schema` with
   `Frontmatter`, `Require`, `Sections []Scope`.
   Each `Scope` carries `Heading`, `Required`,
   `Aliases`, `Sections` (recursive),
   `Repeats`, `Sequential`, `Min`, `Max`,
   `Closed`, and `Rules`. A `"..."` entry
   parses to a `Scope` with `Wildcard: true`.
2. Build two parsers feeding the same struct:
   inline (YAML under `kinds.<name>.schema:`)
   and file (`proto.md` extended).
3. Reject configs that set both inline and
   file sources on one kind.
4. Re-implement MDS020 on top of the schema
   engine. Today's heading-template behavior
   becomes a `sections:` list with no
   `repeats:` and no nested `sections:`;
   every existing fixture passes unchanged.
5. Implement the recursive section-tree
   validator: presence, aliases, repeating
   matches with `sequential:` / `min:` /
   `max:`, recursion into nested `sections:`,
   open-vs-closed scope handling, and
   `"..."` wildcard slots. Levels come from
   tree depth; mismatched document levels
   produce a diagnostic. Diagnostics use plan
   147's shape.
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
- [ ] A schema with a nested section tree
      validates presence, aliases, repeating
      matches (`repeats:` + `sequential:` +
      `min:`), and recursion to at least
      three levels of depth on a runbook
      fixture.
- [ ] A scope without `closed:` allows
      unlisted headings between listed
      sections (regression: a runbook with
      one extra `## Notes` section between
      `## Symptoms` and `## Diagnosis`
      passes).
- [ ] `closed: true` flags an unlisted
      heading and names it.
- [ ] A `"..."` wildcard slot tolerates
      unknown headings at that position even
      under `closed: true`, while enforcing
      surrounding listed sections' order.
- [ ] Mismatched heading depths flag a
      diagnostic naming expected vs actual
      levels.
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
