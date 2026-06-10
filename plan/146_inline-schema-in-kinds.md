---
id: 146
title: Schema engine — sources, scope tree, per-scope rules
status: "✅"
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
        - heading: "Step"
          required: true
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

- `heading:` — string (literal heading text),
  `null` (preamble), or `{unlisted: true}`
  (slot). The level for string headings comes
  from depth.
- `required:` — default `true`.
- `aliases:` — alternate heading texts. Not
  allowed on preamble or slot entries.
- `sections:` — nested sections one level
  deeper. Not allowed on preamble entries.
- `closed:` — per-scope strictness toggle.
- `rules:` — per-scope rule overrides.

The repeating-pattern keys live on the Scope
struct. The inline parser rejects them at
parse time. A future plan will lift the
rejection once repeating-section enforcement
ships.

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

`closed:` is per-scope. A strict root with
permissive subsections sets `closed: true` at
the root and omits it on each child.

> **Entry shape superseded by plan 2606022124.** The
> grammar below is historical; the current shape
> lives in the
> [section-schema reference](../docs/reference/section-schema.md).

Every section-array entry sets `heading:`. The
value is a string (literal heading text), `null`
(the preamble — content before any heading),
or a mapping (typed match). Today the only
mapping form is `{unlisted: true}`. The
`heading: null` preamble is only valid as the
first entry:

```yaml
schema:
  closed: true
  sections:
    - heading: null
      required: false
    - heading: "Overview"
    - heading: {unlisted: true}
    - heading: "References"
```

The schema accepts a preamble before any heading.
Requires Overview first. References last. Other
unlisted sections may appear between Overview and
References (the slot absorbs them). A heading
whose text matches a later listed scope is still
claimed as out-of-order, not absorbed by the
slot — so the slot only covers truly-unlisted
sections.

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
Plan 146 stacks the override on top of the
rule's defaults; threading the full
defaults → kinds → file globs → scope merge
through the engine is a tracked follow-up.

## Tasks

1. ✅ Define `internal/schema/Schema` and its
   recursive `Scope` (Heading, Required,
   Aliases, Sections, Repeats, Sequential, Min,
   Max, Closed, Wildcard, Rules).
2. ✅ Two parsers feed the same struct: inline
   YAML under `kinds.<name>.schema:` and a
   `proto.md` file. Repeating-pattern keys
   (`repeats` / `sequential` / `min` / `max`)
   live on the Scope struct but the inline
   parser rejects them at parse time; the
   rejection waits on a future plan that
   ships repeating-section enforcement.
3. ✅ Reject configs that set both sources on
   one kind (see
   `internal/config/validate.go`).
4. ✅ MDS020 uses the schema engine for inline
   schemas. The legacy file-based path stays so
   `{field}` heading/body sync survives; both
   paths share diagnostic text.
5. ✅ Recursive validator: presence, aliases,
   nested `sections:`, open-vs-closed,
   `heading: {unlisted: true}` slots, the
   `heading: null` preamble, and level-mismatch
   detection.
6. ✅ Per-scope rule overrides (minimal): scope
   `rules:` blocks re-run the named rule and
   filter diagnostics to the scope's heading
   range. The override stacks on rule defaults;
   the full
   defaults → kinds → file globs → scope stack
   is a follow-up.
7. ✅ Documented in the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md).
   Added [docs/guides/schemas.md](../docs/guides/schemas.md).
8. ✅ Fixtures: `good/inline-flat.md`,
   `good/inline-runbook.md` (3-level tree with
   aliases), `good/inline-wildcard.md`,
   `bad/inline-missing.md`,
   `bad/inline-closed-unlisted.md`,
   `bad/inline-level-mismatch.md`. The
   per-scope-rule override is covered by
   `TestCheck_InlineSchema_PerScopeRuleOverride`
   because the integration harness configures
   one rule at a time; fixture form is a
   follow-up.

## Acceptance Criteria

- [x] An inline `schema:` block (front matter
      + flat sections) emits the same
      diagnostics as the equivalent
      `proto.md`-referenced kind.
- [x] A schema with a nested section tree
      validates presence, aliases, and
      recursion to at least three levels of
      depth on a runbook fixture. (Repeating-
      match enforcement is deferred to plan
      142.)
- [x] A scope without `closed:` allows
      unlisted headings between listed
      sections (regression: a runbook with
      one extra `## Notes` section between
      `## Symptoms` and `## Diagnosis`
      passes).
- [x] `closed: true` flags an unlisted
      heading and names it.
- [x] A `heading: {unlisted: true}` slot
      tolerates unlisted headings at that
      position even under `closed: true`,
      while still enforcing surrounding
      listed sections' order.
- [x] A `heading: null` preamble entry parses
      only as the first item in a section
      list; later positions or duplicates
      error at load time.
- [x] Mismatched heading depths flag a
      diagnostic naming expected vs actual
      levels.
- [x] A schema `rules:` block on a section
      applies the override to that section
      only (verified with same prose in two
      sections — unit test).
- [x] Setting both `schema:` and
      `rules.required-structure.schema:` on a
      kind produces a config error naming the
      kind and both sources.
- [x] All existing MDS020 fixtures pass
      against the new engine without
      modification.
- [x] The MDS020 README documents the engine
      with one worked example.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
