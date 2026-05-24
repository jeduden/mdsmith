---
summary: >-
  Direction B from the schema-unification spike:
  a single YAML DSL for the whole schema (front
  matter + body), with a `cue:` escape valve for
  the small set of FM constraints that need
  CUE's full power. This document walks the
  surface end to end with worked examples and
  spells out how each existing CUE shape
  translates.
---
# Direction B: YAML DSL with CUE escape

This is the deep dive of
[Direction B from the spike](spike.md). It
spells out the surface, walks five worked
schemas (RFC, runbook, plan, deck, ADR), shows
how each existing CUE constraint translates,
and traces the validator end-to-end through one
real example.

## Why Direction B

The
[spike](spike.md)
recommends Direction B with a CUE escape valve.
The argument:

- One language for the whole schema — one
  syntax, one doc page, one diagnostic style.
- YAML is the modal config format in LLM
  training data. An agent's first guess at
  "type a schema for me" is more often valid
  YAML than valid CUE.
- The CUE escape (`cue: <expr>`) keeps the
  power of CUE for the rare cases that need
  it: cross-field constraints, kind unification,
  the long tail.
- Body validation is a green-field design (no
  off-the-shelf language solves it). YAML lets
  us pick the shape that fits Markdown, not
  fit the shape some external schema language
  imposes.

This document is the design that follows from
that recommendation.

## The DSL at a glance

A schema has six top-level keys, all optional:

| Key                 | Purpose                            |
| ------------------- | ---------------------------------- |
| `frontmatter:`      | FM field constraints               |
| `require:`          | Filename / path / kind constraints |
| `sections:`         | Recursive section tree (plan 146)  |
| `cross-references:` | Text-pattern-to-heading checks     |
| `acronyms:`         | First-use detection with safe-list |
| `index:`            | JSON side-output                   |

Plus three modifiers that attach anywhere:

- `extends:` — schema inheritance (plan 135)
- `closed:` — strict mode (plan 146)
- `rules:` — per-scope rule overrides (plan 146)

## Front-matter constraints

### Bare-name shortcuts

Every constraint is a YAML shape. The simplest
form is a bare type name:

```yaml
schema:
  frontmatter:
    id: string
    created: date
    homepage: url
    is-draft: bool
    priority: int
```

Type names: `string`, `int`, `number`, `bool`,
`date`, `datetime`, `time`, `email`, `url`,
`filename`, `slug`, `uuid`, `non-empty`. The
list extends per plan 148.

Optional fields use a `?` suffix:

```yaml
frontmatter:
  due?: date
```

### Constraint maps

When a field needs more than a bare type, the
value is a map. A `type:` key disambiguates the
map form from a CUE escape:

```yaml
frontmatter:
  status:
    type: string
    enum: [draft, ratified, deprecated]

  id:
    type: string
    pattern: '^RFC-[0-9]{4}$'

  priority:
    type: int
    min: 1
    max: 5

  authors:
    type: list
    items: string
    min-length: 1

  contact:
    type: email
    deprecated: true
    replaced-by: owner
```

Recognized map keys (per type):

| Type                           | Keys                                                                 |
| ------------------------------ | -------------------------------------------------------------------- |
| any                            | `type`, `optional`, `description`, `deprecated`, `replaced-by`       |
| `string` / `email` / `url` / … | `pattern` (regex), `enum`, `min-length`, `max-length`                |
| `int` / `number`               | `min`, `max`, `multiple-of`, `enum`                                  |
| `bool`                         | (none beyond common)                                                 |
| `date` / `datetime` / `time`   | `min`, `max` (ISO strings or `now`, `now-7d`, `now+1y`)              |
| `list`                         | `items` (recursive type), `min-length`, `max-length`, `unique: true` |
| `map`                          | `keys`, `values` (recursive types)                                   |

`enum:` is the YAML rendering of CUE's
disjunction of literals. `pattern:` is the
YAML rendering of CUE's `=~`. `min` / `max`
cover CUE's range constraints. The 95% case.

### CUE escape

For the remaining cases — cross-field
constraints, recursive unification with another
kind, anything that needs CUE's lattice — a
`cue:` key takes a string holding the CUE
expression:

```yaml
frontmatter:
  status: string
  approver:
    cue: |
      if status == "ratified" {
        != "" & != "draft-bot"
      }
```

The validator unifies the CUE expression
against the field's parsed JSON value. The
diagnostic shape (plan 147) wraps any CUE
error: `field: approver`, `actual: "draft-bot"`,
`expected: <CUE expression rendered>`,
`hint: status="ratified" requires approver != …`.

`cue:` is the only place CUE syntax appears in
the schema. A reader who never encounters a
cross-field constraint never has to learn CUE.

### What translates and what doesn't

Existing `proto.md` schemas in the repo use a
small fraction of CUE. The translations:

| CUE pattern                 | YAML form                                          |
| --------------------------- | -------------------------------------------------- |
| `string`                    | `type: string`                                     |
| `int & >=1 & <=5`           | `type: int, min: 1, max: 5`                        |
| `string & != ""`            | `type: non-empty` or `type: string, min-length: 1` |
| `=~"^FOO-[0-9]+$"`          | `type: string, pattern: '^FOO-[0-9]+$'`            |
| `"a" \| "b" \| "c"`         | `type: string, enum: [a, b, c]`                    |
| `"a" \| "b" \| *"a"`        | `enum: [a, b], default: a`                         |
| `[...string]`               | `type: list, items: string`                        |
| `[...string] & len(x) >= 1` | `type: list, items: string, min-length: 1`         |
| `field?: T`                 | `field?: T`                                        |
| anything cross-field        | `cue:                                              |

A migration tool (out of scope here, candidate
follow-up) walks every `proto.md` in the repo,
emits the YAML equivalent, and flags any field
that requires `cue:` so the maintainer
reviews the escape.

## Sections (plan 146 shape)

Body structure uses the recursive `sections:`
list from plan 146 — heading text, level from
depth, optional `repeats:` for sequence
patterns, `closed:` for strict mode, `"..."`
for wildcard slots:

```yaml
schema:
  sections:
    - heading: "Overview"
      required: true
    - heading: "Decision"
      required: true
      rules:
        paragraph-readability:
          max-readability: 12
    - heading: "Consequences"
      required: true
    - heading: "References"
      required: false
```

This block is unchanged from plan 146's design.
Direction B does not change the body shape; it
unifies the FM shape with the body shape under
one syntax.

## Worked example: ADR

A complete schema for an Architecture Decision
Record:

```yaml
kinds:
  adr:
    path-pattern: "docs/adr/ADR-[0-9][0-9][0-9][0-9].md"
    schema:
      frontmatter:
        id:
          type: string
          pattern: '^ADR-[0-9]{4}$'
        title: non-empty
        status:
          type: string
          enum: [proposed, accepted, deprecated, superseded]
        created: date
        deciders:
          type: list
          items: string
          min-length: 1
        supersedes?:
          type: string
          pattern: '^ADR-[0-9]{4}$'
        # cross-field: superseded ADRs must name a successor
        superseded-by:
          cue: |
            if status == "superseded" { string & =~"^ADR-[0-9]{4}$" }
            if status != "superseded" { *null | string }

      sections:
        - heading: "Context"
          required: true
          rules:
            max-section-length:
              max-words: 300
        - heading: "Decision"
          required: true
        - heading: "Consequences"
          required: true
          sections:
            - heading: "Positive"
              required: false
            - heading: "Negative"
              required: false
        - heading: "Alternatives considered"
          required: false
        - heading: "References"
          required: false
```

Reading top to bottom: filename glob, FM
fields with one CUE escape for the
cross-field rule, the section tree mixing
required and optional sections plus a
nested Consequences group. One language. No
CUE outside the explicit escape.

## Worked example: runbook

The S-7 sketch ported to Direction B:

```yaml
kinds:
  runbook:
    schema:
      frontmatter:
        id: { type: string, pattern: '^RB-[0-9]{4}$' }
        owner: email
        last-tested?: date

      sections:
        - heading: "Overview"
          required: true
          rules:
            max-section-length: { max-words: 200 }

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
                  rules:
                    forbidden-paragraph-starts:
                      starts: ["We ", "The system "]
                - heading: "Expected"
                  required: true
                - heading: "If different"
                  required: false
                  rules:
                    required-text-patterns:
                      patterns:
                        - pattern: 'see Step \d+'
                          message: missing forward reference
                          skip-indices: [-1]

        - heading: "Pass criteria"
          required: true
          rules:
            forbidden-text:
              contains: [should, may, might]

        - heading: "References"
          required: false

      cross-references:
        - pattern: '\bStep (\d+)\b'
          must-match: "Step {n}"
          skip-lines-matching: '^> '

      acronyms:
        known-safe: [API, HTTP, TLS, JSON]
        scope: [Check, Expected]

      index:
        output: ".runbook-index.json"
        include: [step-map, cross-ref-graph, word-counts]
```

This is the entire S-7 example expressed in
the unified DSL. No CUE escape was needed —
the runbook's constraints all fit declarative
shapes.

## Worked example: a plan in this repo

The plan-file schema mdsmith uses on itself,
ported to Direction B:

```yaml
kinds:
  plan:
    path-pattern: "plan/[0-9][0-9]*_*.md"
    schema:
      frontmatter:
        id: { type: int, min: 1 }
        title: non-empty
        status:
          type: string
          enum: ["🔲", "🔳", "✅", "⛔"]
        summary: { type: string, default: "" }
        model:
          type: string
          enum: [haiku, sonnet, opus, ""]
          default: ""
        depends-on:
          type: list
          items: int
          default: []

      sections:
        - heading: "?"   # placeholder vocabulary still applies
          required: true
        - "..."
        - heading: "Goal"
          required: true
        - "..."
        - heading: "Tasks"
          required: true
        - "..."
        - heading: "Acceptance Criteria"
          required: true
        - "..."
```

Compare with today's [`plan/proto.md`](../../../plan/proto.md):
the FM constraints are the same, just expressed
in YAML, and the body structure is the same
heading template.

## Worked example: deck

A presentation-deck schema:

```yaml
kinds:
  deck:
    schema:
      frontmatter:
        title: non-empty
        author: non-empty
        delivered?: date

      sections:
        - heading: "Slide {n}"
          repeats: true
          sequential: true
          min: 3
          rules:
            max-section-length:
              max-words: 60   # decks are terse
            forbidden-paragraph-starts:
              starts: ["TODO", "FIXME"]
```

The schema enforces "every slide is at most 60
words" via plan 142's `max-section-length`
rule, scoped to each `Slide {n}` section
through plan 146's per-scope override.

## Worked example: cross-field constraint

The smallest case where `cue:` is necessary —
"if `archived: true`, `archived-on` is required":

```yaml
frontmatter:
  archived: { type: bool, default: false }
  archived-on?: date

  # Cross-field: when archived, archived-on must
  # be set. Expressed as a CUE constraint that
  # unifies against the FM struct.
  _cross:
    cue: |
      if archived { archived-on: date }
```

A `_cross:` field name is treated specially:
its `cue:` expression unifies against the
whole FM struct rather than one field's value.
Diagnostics from `_cross:` violations name the
struct path (`archived-on`) the CUE engine
reports failure on.

## Diagnostic shape

Every diagnostic from this DSL — declarative
shape *or* CUE escape — flows through plan
147's
`SchemaDiagnostic{Field, Actual, Expected, Hint, SchemaRef}`
and emits as a `lint.Diagnostic` with
`RuleID: "MDS020"`. The user sees one shape;
LSP clients see one wire format.

A declarative-shape failure:

```text
status: got "draft", expected one of:
  proposed, accepted, deprecated, superseded
schema: kinds[adr] / status:6
```

A CUE-escape failure:

```text
superseded-by: got null, expected:
  string matching ^ADR-[0-9]{4}$
hint: status is "superseded"; superseded-by must name an ADR.
schema: kinds[adr] / superseded-by:14 (cue)
```

The `(cue)` suffix on the schema reference
flags that the constraint came from a CUE
escape; otherwise the format is identical.

## End-to-end trace: validating one runbook

Take a runbook claiming `kinds: [runbook]`:

```markdown
---
kinds: [runbook]
id: RB-0042
owner: alice@example.com
---
# Power supply hangs on boot

## Overview
Brief.

## Symptoms
Boot hangs after BIOS post.

## Diagnosis
### Step 1
#### Check
Power LED state.
#### Expected
Solid green.

## References
- [Vendor docs](...)
```

What the validator does, end to end:

1. **Discovery.** The kind matcher finds
   `kinds: [runbook]` in FM and a path glob
   match (none here; assume `kind-assignment`
   covers the file).
2. **Schema load.** The inline `runbook:`
   schema parses to the in-memory `Schema`
   struct from plan 146.
3. **FM validation.** Walk the schema's
   `frontmatter:` block. `id` is `RB-0042`:
   matches `^RB-[0-9]{4}$` ✓. `owner` is a
   valid email ✓. `last-tested?` absent;
   optional ✓.
4. **Section tree.** Walk the AST in lockstep
   with `sections:`. Overview ✓. Symptoms ✓.
   Diagnosis with one `Step {n}` matching ✓.
   Step 1 has Check and Expected; "If
   different" is optional and absent ✓. **Pass
   criteria is required and absent — flag.**
   References ✓.
5. **Cross-references.** Scan text for `\bStep
   (\d+)\b`. The body mentions no step refs
   (that's fine; pattern is "must resolve
   when present"). ✓
6. **Acronyms.** Walk `Check` and `Expected`
   scopes. "BIOS" is all-caps, length 4, not
   in `known-safe`, no parenthesized
   expansion. **Flag.**
7. **Per-scope rules.** `max-section-length:
   max-words: 200` on Overview: "Brief." is
   one word ✓.
   `forbidden-paragraph-starts: ["We ", "The
   system "]` on Check: "Power LED state."
   passes ✓.

Two diagnostics emitted:

```text
section: Pass criteria, expected to be present
hint: insert "## Pass criteria" after "## Diagnosis"
schema: kinds[runbook] / sections[3]

acronym: BIOS, first use without expansion
hint: write "BIOS (Basic Input/Output System)" or add to known-safe
schema: kinds[runbook] / acronyms (scope: Expected)
```

Both flow through `lint.Diagnostic`. The LSP
client highlights the missing-section spot
on line of "## References" (the next-best
anchor) and the BIOS line at its source
position.

## Migration story

Repos with existing `proto.md` schemas keep
working through a compatibility shim:

- The schema loader recognizes both the new
  YAML DSL and the existing CUE-in-FM form.
- A new flag `mdsmith schema migrate <path>`
  rewrites `proto.md` schemas to the YAML DSL,
  emitting a `cue:` escape for any constraint
  it cannot translate.
- The migration is opt-in. Repos that prefer
  to stay on CUE-in-FM continue to do so
  indefinitely.

The migration tool is a candidate plan
(separate from this design doc).

## Open questions

1. **Where does `cue:` actually run?** Two
   options: (a) at validation time, the
   validator unifies the CUE expression
   against the field's parsed JSON; (b) at
   load time, the YAML DSL is compiled into
   a CUE schema and CUE does all validation.
   (a) is simpler; (b) lets `cue:` cross-
   reference declarative-shape fields. Pick
   one; document the trade-off.
2. **Inheritance and the CUE escape.** When a
   child schema overrides a field that the
   parent set with a `cue:` escape, what
   wins? Suggest: the child's expression
   wholly replaces the parent's, same as
   plan 135's CUE-side semantics.
3. **YAML schema for the DSL itself.** The
   DSL is itself YAML; we should publish a
   JSON Schema or YAML schema describing the
   DSL so editors get completion. Likely
   self-hosted: the DSL describes itself.
4. **Validation of `_cross:`.** A `_cross:`
   field that refers to non-existent FM keys
   should fail at load time, not at the
   first violating file. The validator
   needs a static check pass.
5. **Performance of the CUE escape.** Each
   `cue:` expression compiles once at load,
   evaluates per file. Cache the compiled
   form; measure on a 5k-file workspace
   before shipping.
6. **What exactly goes in `cue:` blocks?**
   Should they be raw CUE, or a slightly
   cut-down dialect that mdsmith promises to
   keep compatible? Raw CUE is simpler;
   cut-down is safer. Pick one.

## What folds back into the plans

- **Plan 146** keeps its body design intact
  (sections, repeats, closed/open, wildcard,
  per-scope rules). FM section gains the
  YAML shape — bare names, constraint maps,
  `cue:` escape.
- **Plan 148** (named field-type shortcuts)
  becomes the bare-name vocabulary in the
  YAML DSL. The CUE library is still
  available as the implementation backing
  but is no longer the user surface.
- **Plan 135** (`extends:`) operates on the
  YAML schema struct, with the same
  child-replaces-parent / refinement-on-
  unify semantics.
- **Plan 136** (`deprecated:`) attaches as
  a constraint-map key.
- **Plans 142, 143** are unchanged in shape;
  their per-scope `rules:` config flows
  through the unified DSL.

## See also

- [Spike — schema-language unification](spike.md)
- [Languages survey](languages-survey.md)
- Plans
  [146](../../../plan/146_inline-schema-in-kinds.md),
  [147](../../../plan/147_actionable-schema-diagnostics.md),
  [148](../../../plan/148_named-field-type-shortcuts.md),
  [135](../../../plan/135_schema-extends.md),
  [136](../../../plan/136_field-deprecation-flag.md),
  [142](../../../plan/142_schema-content-constraints.md),
  [143](../../../plan/143_schema-cross-refs-acronyms-index.md).
