---
id: 135
title: Schema inheritance via `extends`
status: "🔲"
model: sonnet
depends-on: [146, 147]
summary: >-
  Let one kind's schema extend another so common
  front-matter fields and structure templates live
  in a base. Single-parent inheritance with
  child-wins semantics, applied to both inline
  (plan 146) and file schemas. Errors point at the
  layer that introduced a conflict.
---
# Schema inheritance via `extends`

## Goal

Let a kind's schema build on a base schema. A
project with three RFC variants (draft, ratified,
deprecated) should declare the shared fields once
in a `rfc-base` schema and only the differences
in the variants.

## Background

The mdbase research records this as **S-3**.

CUE already supports unification. A `proto.md`
schema can compose via `import` plus struct
embedding today. What is missing is an explicit
`extends:` surface that:

- works in inline schemas (plan 146), where
  `import` is not the user's primary tool,
- shows up in `mdsmith kinds` so the inheritance
  chain is visible without reading every schema,
- gives an actionable error (plan 147) when a
  child conflicts with its parent.

## Non-Goals

- Multiple inheritance. Single parent only. CUE
  supports multi-parent unification, but the
  debugging cost is real and the use cases are
  thin. Composition via `<?include?>` covers the
  remaining case.
- Diamond resolution rules. Single inheritance
  makes them moot.
- Schema versioning. A separate concern, tracked
  as **V-1** in the research.

## Design

### `extends:` in a kind

```yaml
kinds:
  rfc-base:
    schema:
      frontmatter:
        id: '=~"^RFC-[0-9]{4}$"'
        authors: '[...string] & len(authors) >= 1'
        created: date
      structure:
        - "# {title}"
        - "## Context"
        - "## Decision"

  rfc-ratified:
    extends: rfc-base
    schema:
      frontmatter:
        ratified-on: date
        status: '"ratified"'
```

Two layers of effect:

- The schema engine **refines** the parent under
  CUE unification: the child's expression for a
  shared key is unified with the parent's, and
  the child must therefore satisfy the parent's
  constraint. A child that cannot unify with
  the parent (parent says `int`, child says
  `string`) is a conflict; a child that
  narrows the parent (parent says
  `"open" | "closed"`, child says `"open"`) is
  a valid refinement.
- The kind's rule overrides also inherit (the
  same deep-merge plan 97 applies). `extends:`
  here is the schema-side surface; the rule-side
  surface remains `kinds:` order plus deep-merge.

### `extends:` in a `proto.md` schema

A file schema declares its parent in front
matter:

```markdown
---
extends: rfc-base.proto.md
---
# {title}
...
```

The path is resolved relative to the schema
file. The loader recurses; cycles are detected
and reported with the cycle path.

### Conflict semantics

`frontmatter:` keys unify under CUE's standard
rules. The effective expression is the unified
form; the child must satisfy the parent.

```yaml
# parent: status: '"open" | "closed"'
# child:  status: '"open"'
# effective: status: '"open"'   (refinement; OK)

# parent: status: '"open" | "closed"'
# child:  status: '"ratified"'
# effective: conflict (no value satisfies both)
```

`structure:` does **not** unify — heading
templates compose by sequence, not by
constraint, so the child's template **replaces**
the parent's wholesale. To extend a parent's
template, copy the parent's lines and add to
them. A future plan can revisit if real cases
need finer-grained merge.

### Surfacing the chain

`mdsmith kinds show <name>` prints:

```text
rfc-ratified:
  extends: rfc-base
  schema: inline
  effective-frontmatter:
    id: =~"^RFC-[0-9]{4}$"      # from rfc-base
    authors: '[...string]'       # from rfc-base
    created: types.#date         # from rfc-base
    ratified-on: types.#date     # from rfc-ratified
    status: '"ratified"'         # from rfc-ratified
```

The provenance column lets a reader see which
layer contributes each field without reading all
schemas. This is the `extends:` analogue of plan
97's deep-merge provenance for rule settings.

### Conflict diagnostics

A child's CUE expression may be unsatisfiable
against its parent's. Parent says `int`; child
says `string`. The diagnostic from plan 147
names both layers:

```text
status: schema cannot unify with parent
  parent rfc-base: '"open" | "closed"'
  child  rfc-ratified: 'int'
schema: kinds[rfc-ratified] / extends[rfc-base]
```

The reader sees the field, both expressions, and
both layer names without grepping.

## Tasks

1. Add an `Extends string` field to the kind
   config struct in `internal/config/`.
2. Add an `extends:` front-matter field to the
   `proto.md` schema parser in
   `internal/rules/requiredstructure/`.
3. Implement parent resolution:

  - inline kinds resolve `extends:` against the
     `kinds:` map by name,
  - file schemas resolve against a path
     relative to the schema file.

4. Detect cycles and report with the full cycle
   path (`a → b → c → a`). Reject before any
   evaluation.
5. Unify child schema onto parent for both
   `frontmatter:` and `require:`. Replace
   wholesale for `structure:`. Reject conflicts
   with the diagnostic shape above (depends on
   plan 147).
6. Extend `mdsmith kinds show <name>` to print
   the inheritance chain and per-field
   provenance for `frontmatter:`.
7. Document inheritance in the
   [file-kinds guide](../docs/guides/file-kinds.md)
   with a worked RFC example.
8. Tests:

  - inline + file schemas each accept
     `extends:` and resolve correctly,
  - a child overrides a parent field; the
     effective schema reflects the child,
  - `structure:` replacement (not merge) is the
     observed behavior, with a regression test,
  - cycle detection produces the expected error
     before any unification runs,
  - conflict diagnostics name both layers.

## Acceptance Criteria

- [ ] An inline kind with `extends:` inherits
      its parent's `frontmatter:` keys; the
      child can override individual keys.
- [ ] A file schema with `extends: <path>`
      inherits identically.
- [ ] A child's `structure:` wholly replaces
      the parent's `structure:` (regression
      test asserts the parent's headings are
      absent from the effective schema).
- [ ] A cycle in `extends:` (single or
      multi-hop) produces an error naming the
      cycle path.
- [ ] A child whose CUE expression cannot unify
      with its parent's produces a diagnostic
      naming both layers (depends on plan 147).
- [ ] `mdsmith kinds show <name>` prints the
      inheritance chain and per-field
      provenance.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
