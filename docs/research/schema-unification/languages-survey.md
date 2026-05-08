---
summary: >-
  Companion to the schema-unification spike.
  Per-language notes on schema languages
  surveyed for typing Markdown front matter
  and body. Each entry covers the language's
  shape, the strongest argument for using it
  for mdsmith's case, the strongest argument
  against, and concrete examples where they
  help.
---
# Schema languages — long-form survey

Companion to
[`spike.md`](spike.md). The spike summarizes;
this document keeps the per-language detail.

## CUE

CUE is a constraint-and-defaults language with
a unification semantics. Front matter at
mdsmith already lives in CUE: regex
(`=~"^FOO-[0-9]+$"`), disjunctions
(`"a" | "b"`), ranges (`int & >=1 & <=5`),
optionality (`field?: T`), and the `*`
preferred-default marker.

**Strengths for tree validation.** Disjunctions
of struct definitions cleanly describe variant
tree nodes — each block type is a closed
struct, and unification picks the matching
shape. Open lists (`[...T]`) and prefix-then-
rest forms (`[#Heading, ...#Block]`) cover
repeating siblings.

**Weaknesses for tree validation.** No
pattern-matching over ordered list contents:
"after every H2 the next non-blank block must
be a paragraph" cannot be stated declaratively.
No recursive predicates: "heading levels never
skip" needs a Go-side pre-pass.
Aggregation/counting is awkward.

**Real users:** Grafana Thema (dashboard
schemas), Timoni (Helm alternative), cfn-cue
(CloudFormation), Istio. None validates a
narrative document body.

**Sources.**
CUE data validation,
[Disjunctions of structs](https://cuelang.org/docs/tour/types/sumstruct/),
[Lists](https://cuelang.org/docs/tour/types/lists/),
[Cuetorials: recursion](https://cuetorials.com/deep-dives/recursion/),
[Grafana Thema](https://github.com/grafana/thema),
[cfn-cue](https://github.com/cue-sh/cfn-cue),
[awesome-cue](https://github.com/xinau/awesome-cue).

## JSON Schema

JSON-based vocabulary for shape constraints.
The de facto interchange format across JS,
Python, and Go for "shape of an object".
Composes via `$ref` and `allOf` / `oneOf`.

**Strengths.** Off-the-shelf integrations for
Markdown front matter:
remark-lint-frontmatter-schema,
eleventy-plugin-collection-schemas,
the [Frontmatter VS Code extension](https://frontmatter.codes).
Universal tooling.

**Weaknesses.** Tree model is poor for ordered
prose; no native way to say "an H2 must be
followed by a paragraph then a code block"
without `oneOf` chains that explode
combinatorially. No cross-cutting predicates
(cf. Schematron).

## TypeSpec (Microsoft)

TypeScript-flavored DSL born in Azure for
HTTP/gRPC API descriptions. Decorators
(`@minLength`, `@pattern`) attach constraints;
emitters compile to OpenAPI / JSON Schema /
Protobuf / SDK code.

**Strengths.** A more pleasant authoring
syntax than raw JSON Schema. Active first-
party investment by Microsoft (used in M365
Copilot tooling in 2025).

**Weaknesses.** Designed around request /
response models, not narrative trees. No body
story. Adopting TypeSpec just for FM means
owning a `.tsp` → JSON Schema build step.

[Overview](https://learn.microsoft.com/en-us/azure/developer/typespec/overview),
TypeSpec for M365 Copilot.

## Pkl (Apple)

Released in 2024 by Apple. Turing-complete
configuration DSL with classes, inheritance,
mixins. Type constraints
(`String(matches(Regex(...)))`). Evaluates to
JSON / YAML / Plist / XML / binary.

**Strengths.** Strong validation expressivity
at leaf-value level. Better tooling story than
CUE for many users. `amends` clean for layered
overrides.

**Weaknesses vs CUE.** Pkl is a programming
language with evaluation, not a constraint
solver — lacks CUE's lattice unification. No
better than CUE on ordered-tree body
validation.

[apple/pkl](https://github.com/apple/pkl),
[Pkl vs CUE discussion](https://github.com/apple/pkl/discussions/7).

## KCL

CNCF Sandbox project. Statically compiled,
schema-oriented constraint language with
single inheritance and `check` blocks for
declarative rules. Pitched as "pragmatic"
where CUE is "dogmatic".

**Strengths.** Schema + `check` arguably
easier than CUE for many shapes. Better
performance on large-scale modeling.

**Weaknesses.** No native ordered-tree model
either. Ecosystem is heavily K8s/policy-leaning.
mdsmith would be the first major Markdown-
tooling adopter.

[KCL intro](https://www.kcl-lang.io/docs/user_docs/getting-started/intro),
[KCL vs CUE](https://github.com/orgs/kcl-lang/discussions/597).

## Dhall

Total (non-Turing-complete, terminating)
functional language. "JSON + functions +
types + imports". `assert` enables value-level
validation at type-check time.

**Strengths.** Termination guarantees and
pure-functional imports — shareable rule packs
that cannot hang the linter. Strong types.

**Weaknesses.** Tiny ecosystem; no Markdown
integrations. Lack of recursion (only
`List/fold` primitives) makes describing
arbitrary AST trees awkward. Termination
already guaranteed by mdsmith's host language.

[dhall-lang.org](https://dhall-lang.org/),
Dhall: validate config.

## RELAX NG / DTD / XSD

Three generations of XML schema languages.
RELAX NG (ISO/IEC 19757-2) is built on finite
tree automata: patterns compose via `<choice>`,
`<interleave>`, `<oneOrMore>`. The compact
syntax (RNC) is genuinely readable.

```rnc
section = element section {
  heading,
  paragraph?,
  oneOrMore(step),
  references?
}
```

**Strengths.** This is the historical proof
that ordered-tree schema languages can be
ergonomic. Lessons: (i) content models are
context-sensitive, not name-sensitive; (ii)
interleave is a real primitive worth having;
(iii) datatype layer separable from
structural layer.

**Weaknesses.** XML syntax is a hard sell in
2026. No built-in cross-document or value-
correlation support. No off-the-shelf RNG-
for-mdast library.

[RELAX NG (Wikipedia)](https://en.wikipedia.org/wiki/RELAX_NG),
[Design of RELAX NG](https://relaxng.org/jclark/design.html),
[Which schema?](https://www.mulberrytech.com/papers/whichschema/contents.html).

## Schematron

ISO/IEC 19757-3. Rule-based, not grammar-
based. An XPath context selects nodes, then
`<assert>` / `<report>` evaluates predicates.

**Strengths.** Designed for cross-cutting
publishing rules. Used in JATS, DITA, TEI for
"if `<install>` then preceding paragraph
mentions OS"-type checks.

**Weaknesses.** XPath/XML syntax. Standalone
adoption is low; the pattern matters more
than the surface — any tree-query language
(JSONPath, CUE selectors, tree-sitter) plays
this role.

**Lesson for mdsmith.** mdsmith already has
the Schematron-half: rules. The MDS020 +
rule-plugins split mirrors RELAX NG +
Schematron. A "single language for both" goal
fights this established split.

[Schematron (Wikipedia)](https://en.wikipedia.org/wiki/Schematron),
RELAX NG + Schematron.

## Existing Markdown body-schema tooling

The most striking finding from this survey:
**no major Markdown tool validates body
structure declaratively.** Body validation is
an open gap industry-wide.

- **markdownlint.** Custom-rule API is JS
  callbacks walking the token stream. Issue
  #22 "Mandatory headings"
  has been open since 2016. No core support
  for "this document must contain these
  sections in this order".
- **Vale.** Closest to "schema for prose" but
  scoped to sentence/paragraph level. No
  extension point for ordered section
  requirements. Tengo scripting is the escape
  hatch; no published Vale package uses it
  for body shape.
- **remark-lint.** ~70 small rules; no
  "required sections in order" rule among
  them. Custom rules walk mdast in JS.
- **Hugo archetypes.** Scaffolding via Go
  templates, not validators. Hugo will build
  a page whose body diverges from its
  archetype. A
  2020 forum thread
  requests schema enforcement; nothing has
  shipped.
- **Eleventy.** `eleventyDataSchema` and
  community plugins (Zod, JSON Schema)
  validate the data cascade — never the
  rendered body.
- **Astro Content Collections.** Zod schemas
  in `src/content/config.ts` validate
  frontmatter only; body is a raw string.
- **Docusaurus.** Joi via
  `@docusaurus/utils-validation` for
  frontmatter; body is not validated.
- **Obsidian Bases.** Explicit: structured
  data lives in YAML frontmatter; body is
  opaque prose.
- **OpenAPI doc generators (Redocly, Slate,
  Widdershins).** Consume the spec and emit
  Markdown templates; do not validate
  hand-written body sections.
- **dprint, mdformat.** Pure formatters, no
  rules engine.
- **mdast spec.** No JSON Schema for mdast
  nodes; TypeScript types only. No tool
  composes mdast-shape constraints into
  whole-tree validation.

The pattern is consistent: **frontmatter gets
schemas; body gets imperative custom rules.**
mdsmith's MDS020 heading-template approach
fills a real gap. The design space for a
richer body schema (sequenced sections,
optional / repeated blocks, content-type
constraints per section) is largely
unexplored in published tooling.

Sources:
markdownlint CustomRules,
[markdownlint #22](https://github.com/DavidAnson/markdownlint/issues/22),
[Vale docs](https://vale.sh/docs),
[remark-lint](https://github.com/remarkjs/remark-lint),
remark-lint custom rule guide,
[Hugo archetypes](https://gohugo.io/content-management/archetypes/),
Hugo schema thread,
[Eleventy data validation](https://www.11ty.dev/docs/data-validate/),
eleventy-plugin-validate,
Astro Content Collections,
[Docusaurus #4591](https://github.com/facebook/docusaurus/issues/4591),
[Obsidian Bases guide](https://chughkabir.com/guide-obsidian-bases/),
[Redocly CLI](https://github.com/Redocly/redocly-cli),
[dprint markdown config](https://dprint.dev/plugins/markdown/config/),
[mdast spec](https://github.com/syntax-tree/mdast),
[MyST spec](https://mystmd.org/spec).

## mdbase

For completeness: mdbase's own answer is a
custom DSL inside Markdown — type definitions
live in `_types/<name>.md`, with type
metadata in the front matter and prose in the
body. Mdbase types do not reach into body
structure; they validate front matter only.
The DSL has 12 named scalar types (string,
int, number, bool, date, datetime, time,
enum, link, …). See the
[mdbase research](../mdbase-vs-mdsmith/learn-from-mdbase.md)
for full detail.
