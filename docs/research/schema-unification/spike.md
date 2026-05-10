---
summary: >-
  Spike investigating whether mdsmith should unify
  the two schema surfaces it has today — CUE for
  front matter, a YAML scope tree for body
  structure (plans 146 / 142 / 143) — into one
  language, and which language(s) best minimize
  the surface area an agent or human must learn
  to type a Markdown document.
---
# Schema language unification — spike

## Why this exists

mdsmith currently uses **two** schema languages
in one schema file:

- **CUE** for front-matter constraints
  (regex, disjunctions, ranges, optionality).
- **A YAML scope tree** for body structure
  (sections, repeats, per-scope rule overrides,
  closed/open) — proposed in
  [plan 146](../../../plan/146_inline-schema-in-kinds.md),
  with content rules in
  [plan 142](../../../plan/142_schema-content-constraints.md)
  and cross-refs / acronyms / index in
  [plan 143](../../../plan/143_schema-cross-refs-acronyms-index.md).

A reader (agent or human) writing a runbook
schema today learns one syntax for FM and a
different syntax for body. Over a project with
ten kinds, the cost of two languages compounds:
two error styles, two doc surfaces, two
mental models of "what a constraint looks
like".

This spike asks: can we get to one language
without losing what each is good at, and what
does the trade-off look like?

The
[mdbase comparison research](../mdbase-vs-mdsmith/learn-from-mdbase.md)
sketches the body-side ambition (S-7) and the
[interop research §7](../mdbase-vs-mdsmith/interop.md)
sketches the schema-bridge problem between
mdsmith and mdbase. Both touch this question
from the side; this spike attacks it head-on.

## What "one language" means

Three candidates for what "one" means:

1. **One syntax** — same notation for both, but
   the notation may have separate sub-vocabularies
   for FM and body.
2. **One semantics** — same evaluator and same
   diagnostic shape, even if the surface
   notation differs (a YAML envelope around
   CUE-ish core, say).
3. **One language** — one parser, one type
   system, one error format, top to bottom.

(3) is the strongest version of the goal but the
hardest. (1) is the easiest. (2) is the
interesting middle. The directions below land
at different points on this spectrum.

## What we want

Concrete properties, ranked roughly by how often
they bite:

1. **Minimal surface area.** A single schema
   author should be able to type "RFC has these
   fields and these sections" without two doc
   pages.
2. **Per-scope rule reuse.** Plan 146 asks
   existing rules to apply per-section without
   code change. The schema language must thread
   rule config through subtrees.
3. **Tree shape with order.** Markdown bodies
   are ordered trees. Headings nest. Repeating
   children (`Step 1`, `Step 2`) need a way to
   say "any number, in sequence".
4. **Cross-cutting predicates.** "Step 4 must
   be followed somewhere by 'see Step ...'."
   Some constraints span multiple subtrees.
5. **FM constraints we have today.** Disjunctions,
   regex, ranges, `?`-optional fields,
   `extends:` (plan 135), shortcuts (plan 148),
   `deprecated:` (plan 136).
6. **Diagnostics.** Plan 147's actionable shape
   — `field`, `actual`, `expected`, `hint`,
   `schema_ref`. New language must hit the same
   bar.
7. **Tooling.** Hover docs, LSP completion,
   error highlighting.
8. **Editor familiarity.** YAML and JSON Schema
   are widely known; CUE less so; Pkl, KCL,
   Dhall less still.

## Inputs

### From the mdbase research

- Schema directions are **S-1** (inline schemas
  in `kinds:`), **S-2** (named field-type
  shortcuts), **S-3** (`extends:`), **S-6**
  (deprecation), **S-7** (rich body schema).
  S-7 is the hard one: nested sections with
  per-field content rules, cross-reference
  validation, acronym tracking, index
  generation. The research explicitly flags the
  schema-language choice as one of S-7's open
  questions: "YAML in `proto.md` front matter
  vs. extending CUE vs. a new DSL".
- The
  [mdbase interop §7](../mdbase-vs-mdsmith/interop.md#7-future-a-schema-bridge)
  sketch points at a translator between
  mdbase's `_types/*.md` DSL and mdsmith's
  CUE-in-`proto.md`. Whatever this spike
  recommends should be friendly to that
  bridge.
- mdbase's own answer is a **DSL inside
  Markdown** — type definitions are themselves
  `_types/<name>.md` files, with type metadata
  in front matter and prose in the body. This
  is a "one syntax" answer for mdbase's
  surface; the type system itself does not
  reach into body structure.

### From the web

The full per-language detail is in
[`languages-survey.md`](languages-survey.md).
The summary that matters here:

- **JSON Schema** is the only schema language
  with off-the-shelf Markdown front-matter
  integrations (remark-lint, eleventy, frontmatter
  VS Code extension). Body validation has no
  comparable adoption. JSON Schema's tree model
  is poor for ordered narrative.
- **CUE** disjunctions and definitions cleanly
  describe variant tree nodes (used by Grafana
  Thema, Timoni, cfn-cue). No native
  pattern-matching over ordered list contents;
  no recursive predicates. Aggregation is
  awkward.
- **Pkl, KCL, Dhall, TypeSpec** all share CUE's
  blind spot on ordered narrative trees.
  Switching to any of them trades constraint
  power for ergonomics, without changing the
  body-validation story.
- **RELAX NG** (especially the compact RNC
  syntax) is the historical proof that
  ordered-tree schema languages can be
  ergonomic. Lessons translate: context-
  sensitive content models beat one-model-
  per-element-name; interleave (unordered) is a
  real primitive worth having; datatype layer
  separable from structural layer.
- **Schematron** is the rule-based companion to
  RNG/XSD: an XPath context selects nodes,
  predicates check them. Used in JATS, DITA,
  TEI for cross-cutting publishing rules. The
  RNG + Schematron split is structurally
  similar to mdsmith's MDS020 + rule-plugin
  split today.
- **No Markdown tool validates body structure
  declaratively.** markdownlint's "Mandatory
  headings" issue
  [#22](https://github.com/DavidAnson/markdownlint/issues/22)
  has been open since 2016. Vale lints
  sentence/paragraph scopes, not section
  shape. Astro / Eleventy / Docusaurus /
  Obsidian Bases all schema **front matter
  only**. Hugo archetypes are scaffolding,
  not validators. mdast has no JSON Schema.
  The pattern is industry-wide:
  **frontmatter gets schemas; body gets
  imperative custom rules**.

The two biggest findings:

1. **No off-the-shelf language solves both
   halves well.** Every choice trades
   something.
2. **mdsmith is in green-field territory on
   body validation.** No "compatible with X"
   constraint pulls toward any particular
   language.

## Directions

Five directions, each illustrated against the
same toy schema — a runbook with Overview,
Diagnosis (repeating Steps), Pass criteria,
References — so they can be compared on equal
footing.

### Direction A — CUE everywhere

Extend CUE to describe the body. Front matter
already lives in CUE; body becomes another CUE
struct.

```cue
package runbook

#Block: #Heading | #Paragraph | #List | #CodeBlock

#Heading: { kind: "heading", level: >=1 & <=6, text: string }
#Paragraph: { kind: "paragraph", inlines: [...string] }

#Section: {
  heading: #Heading
  body: [...#Block]
  sections?: [...#Section]
}

#Runbook: {
  frontmatter: {
    id: =~"^RB-[0-9]{4}$"
    status: "draft" | "ratified"
  }
  sections: [
    { heading: { level: 2, text: "Overview" } },
    { heading: { level: 2, text: "Diagnosis" }
      sections: [...{ heading: { level: 3, text: =~"^Step [0-9]+$" } }]
    },
  ]
}
```

- **A wins on:** one language for the whole schema; one
  evaluator; one diagnostic stream.
- Disjunctions and definitions are excellent for
  variant block types.
- mdsmith already has CUE expertise.

- **A loses on:** CUE has no pattern-match over ordered list
  contents. "After every H2 the next non-blank
  block must be a paragraph" is not directly
  expressible. A real runbook schema needs many
  such constraints; encoding them positionally
  is verbose and brittle.
- No recursive predicates. "Heading levels never
  skip" needs a Go-side pre-pass; the CUE schema
  cannot say it.
- Per-scope rule overrides do not have a natural
  CUE shape — rules are mdsmith concepts, not
  CUE structs. The schema would carry an opaque
  "rules" struct that CUE accepts but does not
  validate.
- Counting and aggregation ("≤ 1 H1") are
  awkward.
- CUE familiarity is low among general
  Markdown users; raising the floor for typing
  one document is the opposite of "minimal
  surface area".

### Direction B — YAML DSL everywhere

Drop CUE. The schema is YAML through and
through. FM constraints become declarative
shapes that compile to a small Go validator.

```yaml
schema:
  frontmatter:
    id:
      type: string
      pattern: '^RB-[0-9]{4}$'
    status:
      enum: [draft, ratified]
  sections:
    - heading: "Overview"
      required: true
    - heading: "Diagnosis"
      required: true
      sections:
        - heading: "Step {n}"
          repeats: true
          sequential: true
          rules:
            paragraph-readability:
              max-readability: 12
```

- **B wins on:** one syntax, one doc page. One error format.
- Most accessible for agents (YAML is the
  lingua franca of LLM-emitted config) and
  humans (most developers can read a YAML
  schema cold).
- Per-scope rule overrides land naturally —
  same shape as the rest of `.mdsmith.yml`.
- The validator is a small Go program;
  diagnostics can match plan 147 exactly with
  no engine quirks to translate.

- **B loses on:** giving up CUE's expressiveness on FM:
  `if/then`, cross-field constraints,
  unification with kind hierarchies. The most
  expressive existing schemas would need to
  re-express in a smaller vocabulary — or use a
  YAML-embedded "expression" subset, which
  starts to look like CUE.
- Drift risk: if the YAML DSL grows over time
  it can drift toward CUE-but-worse.
- mdbase still uses its own DSL; the bridge
  problem (interop §7) becomes "mdsmith YAML
  DSL → mdbase DSL", not visibly easier than
  "CUE → mdbase DSL".

### Direction C — JSON Schema everywhere

JSON Schema for both halves. The schema is a
single JSON document.

- **C wins on:** being the most widely understood schema language.
  Tooling exists for VS Code, JetBrains, the
  CLI, and every major language.
- Off-the-shelf integrations for Markdown FM.
- Unifies with broader documentation tooling
  (OpenAPI, AsyncAPI, Astro Content
  Collections via Zod-to-JSON-Schema).

- **C loses on:** JSON Schema's tree model is poor for ordered
  narrative. Sequential numbering, "an H2 then
  a paragraph then a code block" — these are
  `oneOf`/`allOf` chains that explode.
- No native cross-cutting predicates; you fall
  back to a host language (JS/TS/Python/Go) for
  Schematron-style rules. mdsmith already has
  rules; this is no worse, but it does not
  solve the body problem either.
- JSON Schema is a JSON DSL. The user surface
  is verbose. YAML wrapping helps but
  re-introduces the YAML envelope.

### Direction D — Hybrid with shared primitives

Keep two surfaces (CUE for FM constraints, YAML
scope tree for body) but share definitions: one
**named-type registry**
([plan 148](../../../plan/148_named-field-type-shortcuts.md))
serves both. Inside CUE the registry shows up
as `types.#date`; inside the YAML scope tree as
the shortcut `date`. Same set; different
notation.

```yaml
schema:
  frontmatter:           # CUE under the hood
    id: '=~"^RB-[0-9]{4}$"'
    created: date        # → types.#date
  sections:              # YAML scope tree
    - heading: "Overview"
      required: true
```

- **D wins on:** being the smallest delta from where plans 146–143
  are heading.
- CUE keeps FM expressivity. YAML keeps body
  ergonomics. The shared registry of named
  types prevents drift across the two.
- The schema-bridge sketch (interop §7) maps
  cleanly: types live in one registry, even
  if the structural shapes are translated
  differently per side.

- **D loses on:** still being two languages by syntax, even if the
  semantics share primitives.
- The "per-scope rule overrides" surface lives
  on the YAML side only, so an agent typing
  FM-only constraints doesn't touch it; an
  agent typing body-only doesn't touch CUE.
  The split is real but the cost lands on
  documentation more than on either user
  task individually.

### Direction E — Status quo

Plans 146 / 142 / 143 ship as written. CUE for
FM, YAML for body, no shared primitives beyond
plan 148's named types.

- **E wins on:** smallest implementation cost. Already mostly
  designed.

- **E loses on:** the "one language" pull never settled. A
  team adopting mdsmith reads two docs. An
  agent typing a schema sees two grammars.

## X vs Y deep dives

### Tree shape: CUE recursion vs RNG content models

CUE describes trees via cross-referenced
struct definitions. It can say "a `#Section`
contains a list of `#Block`s, where `#Block` is
one of N variants". It cannot say "a `#Section`
must be: heading, then optional intro
paragraph, then one or more steps" without
spelling out every position.

RNG (RELAX NG) was built precisely to express
context-sensitive content models. It uses
`<sequence>`, `<choice>`, `<oneOrMore>`,
`<interleave>` as primitives. The same runbook
shape:

```rnc
section = element section {
  heading,
  paragraph?,
  oneOrMore(step),
  references?
}
```

reads cleanly and validates fast. The Markdown
analog is `[heading, ?paragraph, +step, ?references]`
in some YAML form. **Lesson:** the body
language wants RNG-style content-model
primitives, not CUE-style recursive structs.
Direction B can pick these up; Direction A
cannot, without bolting on a parallel
mechanism.

### Grammar vs Rules: the Schematron split

XML publishing pipelines (DITA, JATS, TEI)
use **two** schema languages on the same
document: RNG/XSD for grammar, Schematron for
cross-cutting rules. The grammar says "this is
the structure"; the rule layer says "if `<note
type='warning'>` appears in `<install>`, the
preceding paragraph must mention the OS".

mdsmith already has the same split: MDS020
covers grammar, the other 50+ rules cover
cross-cutting checks. The "one language for
both" goal fights this established pattern.
**Lesson:** a unified language for grammar +
rules is rare in practice. The robust choice is
"one language for the grammar, named rules for
the cross-cutting" — which is approximately
what plans 146–143 already encode.

### Surface area: agent vs human

For an LLM emitting a schema, YAML scores
highest by a large margin: it is the modal
config format in training data, the syntax is
forgiving, and the model's first guess at "type
this thing" tends to be valid YAML.

CUE scores lower — agents make more syntax
mistakes (commas vs spaces, struct vs
disjunction, `=~` vs `=`). JSON Schema scores
in between.

For a human reader, the order is
roughly: YAML > JSON Schema > CUE > Pkl/KCL >
RNG. CUE wins on a complex constraint where
its unification semantics save typing; YAML
wins on every shape that fits in one straight
read.

**Lesson:** for the same schema, YAML is the
lower-friction interface for both audiences,
unless the schema needs CUE-specific
expressivity in *every* file. mdsmith's
existing CUE FM schemas don't all need that
expressivity.

### Body validation: build vs adopt

No off-the-shelf language has body validation
for Markdown that matches plan 146's ambition.
Astro's Zod-on-frontmatter is the closest in
the JS ecosystem and stops at FM.

Whatever direction we pick, the body
validator is **mdsmith's to write**. The
choice is which language's notation an author
sees.

## Surface area scoring

How many distinct things must a reader learn
to write a complete schema (FM + body)?

| Direction       | Languages | Doc pages | Diagnostic styles | Existing user familiarity |
|-----------------|-----------|-----------|-------------------|---------------------------|
| A — CUE only    | 1         | 1         | 1                 | low                       |
| B — YAML only   | 1         | 1         | 1                 | high                      |
| C — JSON Schema | 1 (+wrap) | 1         | 1                 | high                      |
| D — CUE + YAML  | 2         | 2         | 1 (shared shape)  | mid (CUE) / high (YAML)   |
| E — status quo  | 2         | 2         | 2                 | mid / high                |

B and D tie on diagnostic style if the FM
side adopts YAML notation around CUE
expressions ("YAML envelope, CUE scalars" —
already what mdsmith does today).

## Recommendation (preliminary)

The strongest argument is for **Direction B
with an escape valve**:

- The default schema surface is YAML through
  and through. Frontmatter constraints get
  declarative shapes (`type:`, `pattern:`,
  `enum:`, `min:`, `max:`, `?`-optional)
  that cover ~95% of existing CUE constraints.
- For the remaining ~5% — cross-field
  constraints, complex unification, kind
  hierarchies — a `cue:` escape hatch lets a
  schema embed a CUE expression as a string.
  The validator unifies the CUE expression
  against the field's parsed value.
- The body side ships as plans 146–143
  describe.

This minimizes surface area (one language)
without losing CUE's power for the rare cases
that need it. It mirrors how Astro's Zod
schemas allow `.refine(fn)` for custom logic
inside an otherwise declarative API.

The fallback recommendation is **Direction
D**: keep both surfaces, add a single named-
type registry that both consult. Less
ambitious but lower risk; this is what plans
148 already buys, so it is also where the
status quo "settles" if no one drives the
unification.

**Direction A (CUE everywhere)** is not
recommended: the gap on ordered-tree
validation is structural, not solvable with
more CUE.

## Open questions

1. **What fraction of existing FM schemas
   actually use CUE's advanced features?** A
   sample of `proto.md` files across mdsmith
   itself and three downstream users would
   answer whether the YAML-DSL shape covers
   the realistic workload.
2. **What does the migration look like?** If
   we adopt Direction B, every existing
   `proto.md` with CUE constraints needs a
   translator. Direction D keeps existing
   schemas working but does not collapse the
   surface.
3. **How does the bridge-to-mdbase
   ([interop §7](../mdbase-vs-mdsmith/interop.md#7-future-a-schema-bridge))
   look under each direction?** The translator
   from mdbase's DSL is simpler against
   Direction B's YAML DSL than against
   Direction A's CUE.
4. **Diagnostics from a CUE escape valve.**
   If Direction B keeps a `cue:` escape
   hatch, the diagnostics from that path need
   to look like the rest of plan 147's
   format. Doable, but needs design work.
5. **Tooling.** LSP hover, completion, and
   error squiggles are easier on a YAML DSL
   than on CUE. The argument for Direction B
   gets stronger when this dimension is
   priced in.

## What folds back into the plans

Once a direction is picked:

- **Plan 146**'s FM section either keeps the
  CUE-shape (Directions A, D, E) or moves to
  the new YAML DSL (Direction B), or wraps
  CUE in YAML (Direction C).
- **Plan 148** (named field-type shortcuts)
  remains in any direction; the shape changes
  per-direction.
- **Plan 135** (`extends:`) is purely a
  schema-side feature; its surface adapts to
  whichever language wins.
- **Plan 142** (content rules) and **plan
  143** (cross-refs / acronyms / index) are
  largely independent of FM language choice;
  their shape lives entirely in the body
  schema.

This spike's job is to gather inputs, not to
land a decision. The next step is to pick a
direction with the project owner, fold the
choice back into plan 146, and adjust 148 /
135 / 142 / 143 to match.

## See also

- [`languages-survey.md`](languages-survey.md)
  — long-form notes on each surveyed language.
- [mdbase research](../mdbase-vs-mdsmith/learn-from-mdbase.md)
  — gaps S-1, S-2, S-3, S-6, S-7.
- [interop §7 schema bridge sketch](../mdbase-vs-mdsmith/interop.md)
  — the cross-tool schema bridge.
- Plans
  [146](../../../plan/146_inline-schema-in-kinds.md),
  [148](../../../plan/148_named-field-type-shortcuts.md),
  [135](../../../plan/135_schema-extends.md),
  [142](../../../plan/142_schema-content-constraints.md),
  [143](../../../plan/143_schema-cross-refs-acronyms-index.md).
