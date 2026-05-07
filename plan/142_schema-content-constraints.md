---
id: 142
title: Schema content constraints
status: "🔲"
model: sonnet
summary: >-
  Add per-section content constraints to the
  schema engine: word and paragraph counts,
  forbidden starts and contents, required
  patterns and mentions. Each maps to one AST
  traversal of the section's subtree. Builds on
  plan 132's scope tree and emits through plan
  133's diagnostic shape.
---
# Schema content constraints

## Goal

Let a schema constrain what goes inside a
section beyond heading shape. A runbook section
with a 50-word cap, a Pass-criteria entry that
must not start with "We" or contain "should",
or a Diagnosis step that must mention a
forward reference — all expressible in the
schema, all enforced at lint time.

This is the **S-7** content-rule slice from the
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md).

## Background

Plan 132 ships the schema engine: scope tree,
sections, per-scope rule overrides. The
existing rule set covers heading-level
concerns. What it does not cover is per-section
content rules — "this section must mention X",
"paragraphs in this section may not begin with
'We'", "this section is at most 50 words".

A few teams write these as custom CI scripts
today. The schema is the right home: it
already knows the section.

## Non-Goals

- Auto-fix for content violations. Detecting
  "first paragraph too long" is fine;
  rewriting it is the user's job.
- Cross-reference resolution. Plan 143.
- Acronym tracking. Plan 143.

## Design

Each constraint attaches to a section scope as
a key on the scope entry:

```yaml
schema:
  sections:
    - heading: "## Pass criteria"
      required: true
      fields:
        - heading: "#### Indicator"
          required: true
          max-words: 30
          forbidden-starts: ["We ", "The system "]
          forbidden-contains: ["should", "may", "might"]

    - heading: "## Diagnosis"
      required: true
      children:
        pattern: "### Step {n}"
        sequential: true
        fields:
          - heading: "#### Check"
            max-words: 50
          - heading: "#### If different"
            required: false
            required-patterns:
              - pattern: "see Step \\d+"
                message: "missing forward reference"
                skip-indices: [-1]
```

### Constraint keys

- `max-words:` and `min-words:` — section body
  word count cap and floor.
- `max-paragraphs:` — cap on paragraph count.
- `forbidden-starts:` — strings; a paragraph
  starting with any of them is flagged.
- `forbidden-contains:` — strings; section
  text containing any is flagged.
- `required-patterns:` — list of
  `{pattern, message, skip-indices}`. A section
  whose body does not match the regex is
  flagged with the configured message.
  `skip-indices:` exempts specific child
  indices ("the last step is exempt"; negative
  indices count from the end).
- `required-mentions:` — strings the section
  must mention at least once.

Each maps to one walk over the section's text
nodes. Word counting reuses the same tokenizer
the existing `paragraph-readability` rule
uses.

### Diagnostics

Every constraint emits through plan 133's
`SchemaDiagnostic` shape. `field` is the
section heading. `actual` is the offending
value: the count, the forbidden string, the
unmatched pattern. `expected` is the
constraint. `schema_ref` points at the schema
location.

### Composition

Constraints stack on the section. A section
with both `max-words: 50` and
`forbidden-contains: ["should"]` runs both
checks; each fires independently.

The constraints run alongside any per-scope
rule overrides from plan 132. A section can
combine `max-words: 50` with a stricter
`paragraph-readability` config; both fire.

## Tasks

1. Extend `internal/schema/Scope` with the
   six constraint keys.
2. Implement the per-section text walk with
   each constraint check. Reuse the
   tokenizer from
   [`internal/lint`](../internal/lint).
3. Compile each `required-patterns:` regex
   at schema-load time; fail with a clear
   error if a pattern is malformed.
4. Diagnostics use plan 133's
   `SchemaDiagnostic` shape with `field` =
   section heading, `actual` = the offending
   value, `expected` = the constraint
   description.
5. Document each constraint in the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md)
   and in `docs/guides/schemas.md`.
6. Add fixtures: a runbook exercising every
   constraint; a `bad/` counterpart that
   trips each.

## Acceptance Criteria

- [ ] `max-words:` and `min-words:` flag
      sections outside the bounds and pass
      sections inside.
- [ ] `forbidden-starts:` flags a paragraph
      starting with the configured string and
      passes other paragraphs.
- [ ] `forbidden-contains:` flags a section
      whose body contains the string.
- [ ] `required-patterns:` flags a section
      whose body does not match the pattern;
      `skip-indices:` exempts the named child
      indices.
- [ ] `required-mentions:` flags a section
      that does not mention the configured
      string.
- [ ] Two constraints on the same section
      both fire when violated.
- [ ] Diagnostics use plan 133's shape with
      the section heading as `field`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
