---
title: Schemas
weight: 30
summary: >-
  Declare a document-structure schema inline on a kind
  or in a proto.md file, validate headings and front
  matter, and tighten rule config per section.
---
# Schemas

A **schema** describes what a Markdown document's
front matter, filename, and heading tree must look
like. Schemas are the engine behind
[MDS020 required-structure](../../internal/rules/MDS020-required-structure/README.md);
they are the canonical place to lock down the shape
of a recurring document type (plan, RFC, runbook,
rule README).

mdsmith reads schemas from two sources:

- **Inline** — a `schema:` block on a kind body in
  `.mdsmith.yml`. Uses the new matcher engine
  (`regex:`, `repeat:`, `\#(digits)`, `\#(fmvar(...))`).
- **File** — a `proto.md` referenced by
  `rules.required-structure.schema:`. MDS020 still
  validates this source through its legacy parser
  today; the schema-package parser shipped with plan
  156 lifts proto.md into the same matcher shape but
  is exercised only by tests until the cutover
  follow-up wires MDS020 through.

A kind may use only one source; setting both is a
config error.

## Inline schemas on kinds

Inline schemas keep the structure declaration next to
the kind's other rule settings. They are best for
small schemas (one or two screens) that do not need
templated body content.

```yaml
kinds:
  rfc:
    schema:
      filename: "RFC-[0-9][0-9][0-9][0-9].md"
      frontmatter:
        id: '=~"^RFC-[0-9]{4}$"'
        status: '"draft" | "ratified" | "deprecated"'
        authors: '[...string] & [_, ...string]'
      closed: true
      sections:
        - heading: null
        - heading: "Overview"
        - heading: "Decision"
        - heading:
            regex: '.+'
            repeat: { min: 0 }
        - heading: "References"
```

The `frontmatter:` mapping reuses CUE expressions per
key: regex, disjunction, list, and any other CUE form
is accepted. Trailing `?` on a key marks it optional.
Named shortcuts —
`date`, `datetime`, `time`, `email`, `url`, `filename`,
`nonEmpty` — substitute for their canonical CUE so a
schema can write `created: date` instead of repeating
the ISO regex; see
[Schema field types](../reference/schema-types.md)
for the registered names and how they are matched.

`filename:` is a glob the document basename must
match. It sits at the top of the schema block (no
`require:` wrapper).

`closed: true` makes the scope strict — unlisted
headings produce a diagnostic. `closed: false` (the
default) tolerates unlisted headings between listed
sections. `closed:` is only meaningful when the
schema declares `sections:`; setting it on a
frontmatter-only kind is a parse error.

### The `heading:` discriminator

Every section-array entry sets `heading:`. The value
takes one of three shapes:

- **`null`** — the preamble: content from line 1 up
  to the first heading. Only valid as the first entry
  in a section list. Carries `closed:` / `rules:` /
  `content:` for that range; rejects `sections:`.
- **string** — sugar for a literal match. The string
  is regex-escaped and used as the matcher's pattern,
  so `heading: "(WIP)"` matches a heading whose text
  is exactly `(WIP)` with the parens taken literally.
  Cardinality is one.
- **mapping** — the full form:
  `{ regex, repeat?, sequential? }`. `regex:` is
  required; the body is a Go RE2 pattern that
  accepts two interpolation references —
  `\#(digits)` and `\#(fmvar(name))`. `repeat:`
  bounds the run; `sequential:` (with `digits`)
  asserts ordering. See the
  [section-schema reference](../reference/section-schema.md)
  for the full grammar.

### The matcher mapping

```yaml
sections:
  - heading:
      regex: 'Step \#(digits)'
      repeat: { min: 1, max: 5 }
      sequential: true
    sections: [...]
    content: [...]
  - heading:
      regex: '\#(fmvar(id)): \#(fmvar(name))'
  - heading:
      regex: '.+'
      repeat: { min: 0 }
```

`regex:` is whole-string anchored against the
heading's rendered plain text (inline emphasis stripped,
link wrappers unwrapped, code-span backticks dropped).
Backslashes pass through to RE2; interpolation uses
`\#(expr)`. Two helpers are in scope:

- **`digits`** — expands to the named capture
  `(?P<n>[0-9]+)`. One per pattern. With
  `sequential: true` the validator asserts the
  captured numbers are strictly increasing without
  gaps.
- **`fmvar(name)`** — looks up the document's
  frontmatter field `name`, regex-escapes its value,
  and substitutes it.

`repeat:` bounds how many consecutive matching
headings the matcher claims. Omitting `repeat:`
means exactly one; `{ min: 0 }` is zero-or-more;
`{ min: 0, max: 1 }` is optional; `{ min: 1 }` is
one-or-more; bounded forms enforce both bounds.
`repeat: { max: 0 }` and `repeat: { min > max }`
each parse-error.

The wildcard-slot shape — `regex: '.+'` with
`repeat: { min: 0 }` — is positional: it absorbs
zero or more unlisted sections at its slot. A
heading whose text matches a later listed entry is
claimed for that entry, not absorbed by the slot.

### Nested sections

Levels come from depth. Root `sections:` entries are
H2; nested `sections:` lists are H3, H4, …. A runbook
that wants Diagnosis → Step → Check / Expected
expresses that as:

```yaml
sections:
  - heading:
      regex: 'Symptoms|Indicators'
  - heading: "Diagnosis"
    sections:
      - heading: "Step"
        sections:
          - heading: "Check"
          - heading: "Expected"
          - heading:
              regex: 'If different'
              repeat: { min: 0, max: 1 }
  - heading:
      regex: 'References'
      repeat: { min: 0, max: 1 }
```

A scope that accepts alternate heading texts encodes
the disjunction in its regex: `regex: 'A|B'` matches
a heading whose text is `A` or `B`.

### Section content

`sections:` constrains nested headings. To pin down
what AST nodes must appear inside a section's body —
a required YAML code block, a settings table with
specific columns, an ordered list with a minimum item
count — add a `content:` list alongside the scope's
existing fields.

```yaml
sections:
  - heading: "Examples"
    closed: true
    content:
      - kind: code-block
        lang: yaml
      - kind: unlisted
```

Each entry sets `kind:` and a small set of optional
kind-specific fields:

- `kind: code-block` — `lang:` constrains the fenced
  block's info string (exact match).
- `kind: table` — `columns:` is the exact header row
  the GFM table must carry.
- `kind: list` — `ordered:` (`true` / `false`),
  `min-items:`, `max-items:` bound the list's shape.
- `kind: paragraph` — no extra keys.
- `kind: unlisted` — a positional slot. Tolerates
  any non-matching nodes at that position even under
  `closed: true`.

Entries match in declared order. A node that
appears earlier than expected but matches a later
listed entry is claimed out-of-order with a
diagnostic — the same rule the heading-tree walker
uses. Sub-shape mismatches (wrong code-block
language, wrong table columns, list violating
ordered/min/max) emit their own diagnostics but
still consume the slot. Missing required entries
anchor at the section's heading line.

`content:` is rejected on a slot scope (the
wildcard-slot shape has no fixed identity to
constrain). Set `content:` only on entries that
match named sections.

### Per-scope rule overrides

Any scope may carry a `rules:` block. The override
sits on top of the rule's defaults — keys it sets
replace the defaults wholesale; keys it omits keep
their default value. The override applies only inside
that scope's heading range. This is the way to say
"this section is stricter than the rest of the
document" without scattering glob overrides.

```yaml
sections:
  - heading: "Decision"
    rules:
      paragraph-readability:
        max-index: 12.0
      max-section-length:
        max-words: 200
```

Two follow-ups land later, tracked on plan 146:
first, the override is not yet a config-style deep
merge — nested maps and list merge modes (e.g.
`placeholders` append) behave like a plain
ApplySettings call. Second, the override stacks on
rule defaults, not the rule's full per-file config;
threading the full
defaults → kinds → file globs → scope merge through
the engine is the same follow-up.

If a scope's `rules:` block names a rule that does
not exist or supplies settings the rule rejects, the
override surfaces as an MDS020 diagnostic at the
scope's heading line.

### Content constraints

Five rules ship per-scope prose constraints. Each is
default-disabled and reuses the standard `rules:`
surface — there is no separate schema vocabulary for
"max words" or "forbidden text". Set them globally
under top-level `rules:` for document-wide
enforcement, or under a scope's `rules:` block for
section-scoped enforcement.

| Rule                                                                                                  | Setting                                        | Effect                                                                |
|-------------------------------------------------------------------------------------------------------|------------------------------------------------|-----------------------------------------------------------------------|
| [MDS036 max-section-length](../../internal/rules/MDS036-max-section-length/README.md)                 | `max-words`, `min-words`, `max-paragraphs`     | Cap word counts and paragraph counts in addition to today's line cap. |
| [MDS055 forbidden-paragraph-starts](../../internal/rules/MDS055-forbidden-paragraph-starts/README.md) | `starts: [str, ...]`                           | Flag paragraphs that begin with any listed prefix.                    |
| [MDS056 forbidden-text](../../internal/rules/MDS056-forbidden-text/README.md)                         | `contains: [str, ...]`                         | Flag paragraphs whose text contains any listed substring.             |
| [MDS057 required-text-patterns](../../internal/rules/MDS057-required-text-patterns/README.md)         | `patterns: [{pattern, message, skip-indices}]` | Flag a section whose body does not match every configured regex.      |
| [MDS058 required-mentions](../../internal/rules/MDS058-required-mentions/README.md)                   | `mentions: [str, ...]`                         | Flag a section that does not contain every listed substring.          |

Document-wide example:

```yaml
rules:
  required-mentions:
    mentions: ["scope: production"]
  forbidden-text:
    contains: ["should", "may", "might"]
```

Per-section example, scoped to a `Diagnosis` section:

```yaml
kinds:
  runbook:
    schema:
      sections:
        - heading: "Diagnosis"
          rules:
            forbidden-text:
              contains: ["should", "may"]
            required-mentions:
              mentions: ["forward reference"]
```

MDS057 and MDS058 anchor diagnostics at the section's
heading line. MDS055 and MDS056 anchor at the
offending paragraph's line. In both cases the
per-scope filter keeps only diagnostics inside the
section's range, so the same rule code works for
document-wide and per-section enforcement.

`skip-indices:` on MDS057 parses but currently does
nothing; it activates when section-content `children:`
ships in a later plan.

### Cross-references, acronyms, and index

Three top-level schema blocks add document-wide
checks and a JSON side-output:

```yaml
kinds:
  runbook:
    schema:
      cross-references:
        - pattern: "\\bStep (\\d+)\\b"
          must-match: "Step {n}"
          skip-lines-matching: "^> "
      acronyms:
        known-safe: [API, HTTP, TLS, JSON]
        scope: ["Check", "Expected"]
      index:
        output: ".runbook-index.json"
        include: [step-map, cross-ref-graph, word-counts, headings]
```

`cross-references:` checks that every match of
`pattern:` in the document body resolves to a heading
slug after filling captures into `must-match:`. Use
`{n}` for the first capture, `{1}` / `{2}` for
numbered captures, or a named group name. The
`skip-lines-matching:` regex exempts blockquoted or
historical lines.

`acronyms:` flags first-use all-caps tokens
(length 2-6) that lack a parenthesised expansion.
`known-safe:` is the allowlist. `scope:` restricts
the check to sections whose heading text matches one
of the listed names; omitting `scope:` applies the
check document-wide.

`index:` asks `mdsmith fix` to write a JSON side-
output next to the source file describing requested
sub-objects (`step-map`, `cross-ref-graph`,
`word-counts`, `headings`). `mdsmith check` never
writes the file. Output paths are resolved relative
to the source file's directory; absolute paths and
`..` traversal are rejected. See
[MDS020 required-structure](../../internal/rules/MDS020-required-structure/README.md#index-side-output)
for the JSON shape per include entry.

Today the scope override runs the rule again with
the override's settings, in addition to the engine's
normal run with the file-level settings. If the same
rule is enabled globally and the override only
tightens the cap, both runs may fire on the same
line — producing two diagnostics where the user
expected one. Workaround until engine-level dispatch
lands: disable the rule globally for files that rely
on the scope override (a kind or override that sets
`rule-name: false` plus the scope's per-section
override).

## File-based schemas (`proto.md`)

A `proto.md` schema is a Markdown file whose headings
describe required structure and whose front matter
holds CUE constraints. This form is best for larger
schemas, schemas that want to template a body, or
schemas reused across kinds via `<?include?>`.

```markdown
---
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
---
<?require
filename: "MDS*-*.md"
?>

# {id}: {name}

## Settings

## Examples

### Good

### Bad
```

The `# ?` (or `# {field}: {field}` form) acts as the
title placeholder. `## ...` rows mark wildcard slots.
Front-matter keys map directly to CUE expressions.
`<?require?>` declares the filename pattern.

MDS020's file-schema check routes through its
legacy parser: `{field}` in a proto.md heading row
matches a non-empty run rather than resolving the
document's frontmatter value via `fmvar(...)`.
Heading rows are wildcards, not substitutions.

`{field}` in a proto.md **body** is fully wired:
MDS020 resolves the placeholder against the
document's front matter and flags any mismatch.
`mdsmith fix` rewrites stale body lines to the
current front-matter value for files that match a
**single file-based schema source**. Composed or
multi-source schemas do not get Fix body rewrites.
The rule-readme `Meta-Information` body uses this
to keep `ID`, `Name`, `Status`, and `Category`
bullets in sync with front matter.

The
[section-schema reference](../reference/section-schema.md#protomd-file-syntax)
records the heading-row wildcard mapping.

## Choosing a source

| Need                                   | Inline | File               |
|----------------------------------------|--------|--------------------|
| Short schema with no templated body    | yes    | works              |
| Schema reused via `<?include?>`        | no     | yes                |
| Frontmatter-body `{field}` sync        | no     | yes                |
| Nested section tree                    | yes    | via heading levels |
| Per-scope rule overrides               | yes    | no                 |
| Stays next to other kind rule settings | yes    | indirect           |

A project can mix sources across kinds — some kinds use
inline schemas, others use `proto.md` — but a single
kind must pick one.

## Schema inheritance with `extends`

A kind can build on another kind's schema by setting
`extends: <parent-name>` next to its `schema:` block.
Frontmatter constraints unify under CUE refinement: a
child that re-declares a parent key joins both with `&`,
so the effective constraint is the intersection. A
child's `sections:` list wholly replaces the parent's,
so heading templates compose by sequence rather than by
constraint. Filename and other document-wide blocks
inherit when the child does not set them.

A `proto.md` file schema declares the same relationship
via an `extends: <path>` key in its front matter; the
path is resolved relative to the schema file with the
same `..`-traversal and absolute-path guards used by
`<?include?>`.

See [Schema inheritance with `extends`](file-kinds.md#schema-inheritance-with-extends)
for the worked RFC example, conflict semantics, and the
`mdsmith kinds show` audit surface.

## Composition across kinds

A file resolved by multiple kinds that each declare a
`required-structure` schema gets the composition of all
of them — not just the last one. The merge layer
accumulates each kind's `schema:` or `inline-schema:`
into a `schema-sources` list, and MDS020 loads every
source and composes them at check time.

The composition rules are:

- **Frontmatter** keys union across schemas. A key
  required by any input is required. Two schemas
  constraining the same key get the intersection of
  their CUE expressions (joined with `&`).
- **Sections** merge by literal heading text. Scopes
  that share the same heading combine their child
  lists recursively. Scopes that differ — including
  wildcard slots (`{unlisted: true}`), the preamble
  (`null`), and the bare `?` wildcard — append in
  input order.
- **`closed:`** is OR-ed across inputs. Any scope that
  was strict in any input is strict in the composed
  scope.
- **`require.filename`** picks the first non-empty
  pattern. Conflicting patterns are a config error.

### Worked example: directive-rule-readme + rule-readme

The four directive READMEs in this repository
(`MDS019-catalog`, `MDS021-include`, `MDS038-toc`,
`MDS039-build`) resolve to both `rule-readme` and
`directive-rule-readme`. The first kind contributes
the common rule-README structure (`Config`,
`Examples`, `Meta-Information`); the second only adds
a required `Pattern` section.

```yaml
kinds:
  rule-readme:
    rules:
      required-structure:
        schema: internal/rules/proto.md
  directive-rule-readme:
    rules:
      required-structure:
        schema: internal/rules/directive-proto.md

kind-assignment:
  - glob: ["internal/rules/MDS*/README.md"]
    kinds: [rule-readme]
  - glob: ["internal/rules/MDS019-catalog/README.md", …]
    kinds: [directive-rule-readme]
```

`directive-proto.md` declares only what's specific to
directive rules:

```markdown
---
nature: '"directive"'
---
# {id}: {name}

## ...

## Pattern

### Without the directive
### With the directive

## ...
```

The composed schema requires the union of both
sections lists. `rule-readme`'s `nature` is
`"directive" | "generator" | "content" | "style" |
"structure"`; `directive-rule-readme`'s narrower
`"directive"` intersects to require exactly
`"directive"` on every file resolving to both kinds.

### Picking an input order

The composed section list is the concatenation of each
schema's sections (with same-heading scopes merged).
Order matters for the "last required section" — if the
later schema's required sections must appear before
the earlier schema's required sections in the document,
either reorder the kinds in `kind-assignment` or rewrite
the document so the sections fall in composed order.
The directive READMEs put `Pattern` after
`Meta-Information` precisely so the composed ordering
(`rule-readme` first, `directive-rule-readme` appended)
matches the document layout.

## Extracting data

A schema doubles as an extraction contract. Once
`mdsmith check` confirms a file conforms,

```bash
mdsmith extract <kind> --format json|yaml|msgpack <file>
```

emits a data tree whose nesting mirrors the schema
hierarchy — no annotations required. The root carries a
`frontmatter` object plus the projected sections beside
it; literal headings key by slug, repeating sections
become arrays whose elements retain every captured
placeholder, a `heading: null` section's content hoists
into its enclosing object, and `code-block` / `list` /
`table` / `paragraph` content entries project under
`code` / `items` / `rows` / `text`. Wildcard slots and
unlisted headings are skipped. See
[`mdsmith extract`](../reference/cli/extract.md) for the
full projection rules and exit codes.

## Diagnostics

Schema diagnostics surface through
[MDS020 required-structure](../../internal/rules/MDS020-required-structure/README.md).
The message text is the same regardless of source, so
this is the place to look up what `missing required
section`, `unexpected section`, `heading level
mismatch`, and `out of order` mean.

## See also

- [Section schema reference](../reference/section-schema.md)
  — the entry-shape grammar in full.
- [File kinds](file-kinds.md) — how kinds attach
  schemas (and other rule config) to file groups.
- [Enforcing document structure with schemas](directives/enforcing-structure.md)
  — the file-based reference.
- [Placeholder grammar](../background/concepts/placeholder-grammar.md)
  — opt-in tokens for template-friendly source.
