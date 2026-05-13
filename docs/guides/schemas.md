---
title: Schemas
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

mdsmith reads schemas from two sources that share one
in-memory representation:

- **Inline** — a `schema:` block on a kind body in
  `.mdsmith.yml`.
- **File** — a `proto.md` referenced by
  `rules.required-structure.schema:`.

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
      frontmatter:
        id: '=~"^RFC-[0-9]{4}$"'
        status: '"draft" | "ratified" | "deprecated"'
        authors: '[...string] & len(authors) >= 1'
      require:
        filename: "RFC-[0-9][0-9][0-9][0-9].md"
      closed: true
      sections:
        - heading: null
          required: false
        - heading: "Overview"
          required: true
        - heading: "Decision"
          required: true
        - heading:
            unlisted: true
        - heading: "References"
          required: true
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

`require.filename:` is a glob the document basename
must match.

`closed: true` makes the scope strict — unlisted
headings produce a diagnostic. `closed: false` (the
default) tolerates unlisted headings between listed
sections.

### The `heading:` field

Every section-array entry sets `heading:`. The value
takes one of three shapes:

- **string** — literal heading text. Example:
  `heading: "Overview"`. The doc must have a heading
  whose text equals that string (or an alias).
- **`null`** — the preamble: content from line 1 up
  to the first heading. Only valid as the first entry
  in a section list. Carries `required:` /
  `closed:` / `rules:` for that range; cannot carry
  `aliases:` or nested `sections:`.
- **mapping** — typed match. Today only
  `{unlisted: true}` is accepted, declaring a slot
  that absorbs zero or more sections the schema did
  not list by name. The slot is positional, but
  out-of-order detection still claims a heading whose
  text matches a later listed scope, so the slot only
  absorbs truly-unlisted sections. Slots are
  positional-only — they cannot carry `aliases:`,
  `sections:`, `rules:`, `closed:`, or `required:`;
  the parser rejects those keys. Future work can
  extend the mapping form with shapes like
  `{any: true}` (match any heading text) or
  `{pattern: "..."}` (match a placeholder pattern).

### Nested sections

Levels come from depth. Root `sections:` entries are
H2; nested `sections:` lists are H3, H4, …. A runbook
that wants Diagnosis → Step → Check / Expected
expresses that as:

```yaml
sections:
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

`aliases:` lets a heading match alternate texts. A
required scope that lists `aliases: ["Indicators"]`
matches both `## Symptoms` and `## Indicators` in a
document.

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

`content:` is not accepted on slot scopes
(`heading: {unlisted: true}`) or on the `?`
wildcard heading — the parser rejects those
shapes today.

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
    required: true
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
`<?require?>` declares the filename pattern. See
[enforcing document structure](directives/enforcing-structure.md)
for the full file-based reference.

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

## Diagnostics

Schema diagnostics surface through
[MDS020 required-structure](../../internal/rules/MDS020-required-structure/README.md).
The message text is the same regardless of source, so
this is the place to look up what `missing required
section`, `unexpected section`, `heading level
mismatch`, and `out of order` mean.

## See also

- [File kinds](file-kinds.md) — how kinds attach
  schemas (and other rule config) to file groups.
- [Enforcing document structure with schemas](directives/enforcing-structure.md)
  — the file-based reference.
- [Placeholder grammar](../background/concepts/placeholder-grammar.md)
  — opt-in tokens for template-friendly source.
