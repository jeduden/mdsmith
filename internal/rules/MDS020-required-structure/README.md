---
id: MDS020
name: required-structure
status: ready
description: Document structure and front matter must match its schema.
nature: structure
---
# MDS020: required-structure

Document structure and front matter must match its
schema.

## Settings

| Setting         | Type   | Default | Description                                                                                                                |
|-----------------|--------|---------|----------------------------------------------------------------------------------------------------------------------------|
| `schema`        | string | `""`    | Path to a schema file (a `proto.md`)                                                                                       |
| `inline-schema` | map    | (unset) | Inline schema injected by `kinds.<name>.schema:`; not usually written by hand on a rule. DefaultSettings does not list it. |
| `placeholders`  | list   | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Useful tokens: `cue-frontmatter`.

When neither `schema` nor `inline-schema` is set the
rule skips structure and front matter validation, but
still warns on misplaced `<?require?>` directives. Use
overrides or `kinds:` to apply schemas to specific file
groups.

A kind may declare its schema in either form. The
config loader rejects a kind that sets both — see
[file kinds](../../../docs/guides/file-kinds.md).

Schema front matter may embed a CUE schema that
validates document front matter. The rule-readme
schema at [internal/rules/proto.md](../proto.md)
requires `id`, `name`, `status`, `description`, and
`nature` (one of `directive`, `generator`,
`content`, `style`, `structure`). See the proto
file's leading comment for the vocabulary.

### Require directive

Use `<?require?>` in the schema body to declare
constraints on files validated against this schema:

| Field      | Type   | Description                           |
|------------|--------|---------------------------------------|
| `filename` | string | Glob the document basename must match |

```markdown
<?require
filename: "[0-9]*_*.md"
?>
```

### Schema composition

Schema files can use `<?include?>` to share
structure across schemas. Included fragment
headings are spliced into the heading list at
the include position. Fragment front matter is
ignored. `<?require?>` from fragments is merged.

```markdown
# ?

## Goal

<?include
file: common/acceptance-criteria.md
?>
```

Cycle detection prevents circular includes.
Max include depth is 10.

### Optional fields

Append `?` to a schema front matter key to make it
optional. The field may be absent in the document,
but if present it must satisfy the type constraint:

```yaml
name: 'string & != ""'
"description?": string
```

Schema body controls section strictness:

- By default, extra sections are rejected.
- Add a heading with text `...` (for example `## ...`) to
  allow extra headings in that position until the next
  required heading anchor.

### Inline schemas on kinds

A kind body may declare its schema directly in
`.mdsmith.yml` rather than referencing a `proto.md`
file. The two forms are equivalent — both parse to the
same in-memory scope tree — and a kind may use only one.

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
          sections:
            - heading: "Outcome"
              required: true
        - heading:
            unlisted: true
        - heading: "References"
          required: true
```

Section keys:

- `heading:` — string (literal text), `null` (the
  preamble: content before any heading), or mapping
  (typed match — today only `{unlisted: true}` for a
  slot). The level for string headings comes from
  depth in the tree (root sections are H2; nested
  sections are H3, then H4, …).
- `required:` — defaults to `true`. Preamble entries
  typically set `required: false`.
- `aliases:` — alternate heading texts. Not allowed
  on preamble or slot entries (no name to alias).
- `sections:` — nested sections one level deeper.
  Not allowed on preamble entries (the first heading
  terminates the preamble's range).
- `closed:` — when `true`, unlisted headings inside
  this scope produce a diagnostic. The same flag
  drives the `content:` walker — unlisted body
  nodes outside an `unlisted` content slot also
  flag under `closed: true`. Default `false`.
- `rules:` — per-scope rule-config overrides. Each
  entry maps a rule name to a settings map that
  applies on top of the rule's defaults inside the
  scope's range. Today's apply is a plain
  ApplySettings call, not a config-style deep-merge —
  keys the override sets replace the defaults
  wholesale.
- `content:` — positional list of non-heading
  AST node constraints (code blocks, tables,
  lists, paragraphs) the section must contain.
  See
  [the schemas guide](../../../docs/guides/schemas.md#section-content)
  for the entry shape and the per-kind fields.
  Rejected on slot or `?` wildcard scopes.

A slot entry (`heading: {unlisted: true}`) absorbs
zero or more unlisted sections at that position.
Surrounding listed sections still keep their order.
Out-of-order detection still claims a heading whose
text matches a later listed scope, so the slot only
absorbs truly-unlisted sections.

Slots are positional-only: the parser rejects
`aliases:`, `sections:`, `rules:`, `closed:`, and
`required:` on a slot scope. The preamble
(`heading: null`) accepts `required:`, `closed:`,
and `rules:` for its line range; it rejects
`aliases:` and `sections:`.

The document's H1 is reserved for the title and is
validated by `first-line-heading`; inline schemas
constrain H2 and below.

### Cross-references

A `cross-references:` block names text patterns whose
matches must resolve to a real heading in the document.
Each entry walks the document's inline text once and
fills numeric (`{n}`, `{1}`, `{2}`, …) or named
captures from the regex into the `must-match:` template;
the result is slugified and looked up in the heading
slug set. Unresolved references produce a diagnostic at
the match's source line.

```yaml
schema:
  cross-references:
    - pattern: "\\bStep (\\d+)\\b"
      must-match: "Step {n}"
      skip-lines-matching: "^> "
```

`skip-lines-matching:` is a regex; raw source lines
that match it are exempt from the resolution check.
The intended use is blockquoted stale text and version-
history notes that mention old step numbers.

### Acronyms

An `acronyms:` block flags all-caps tokens (length
2-6, leading letter, alphanumeric) on their first use
inside a configured scope when they appear without a
parenthesised expansion. `known-safe:` lists tokens
allowed without expansion; `scope:` restricts the
check to sections whose heading text matches one of
the listed names. Omitting `scope:` applies the check
document-wide. First-use state is per-scope.

```yaml
schema:
  acronyms:
    known-safe: [API, HTTP, TLS, JSON]
    scope: ["Check", "Expected"]
```

### Index side-output

An `index:` block asks `mdsmith fix` to write a JSON
side-output next to the source file. `mdsmith check`
does not write the file (read-only contract). The
output path is resolved relative to the document's
directory; absolute paths and `..` traversal are
rejected.

```yaml
schema:
  index:
    output: ".runbook-index.json"
    include: [step-map, cross-ref-graph, word-counts, headings]
```

`include:` is a closed enum:

- `step-map` — `{section-slug: [child-slugs]}`
- `cross-ref-graph` — `{ref-text: target-slug}`
- `word-counts` — `{section-slug: int}`
- `headings` — flat list of
  `{level, text, slug, line}`

The set is closed so downstream tooling can parse the
file without a schema reference.

## Config

Apply a schema by declaring a kind or setting
`schema:` on an override:

```yaml
kinds:
  story:
    rules:
      required-structure:
        schema: schemas/story.md

kind-assignment:
  - glob: ["stories/**/*.md"]
    kinds: [story]
```

Disable:

```yaml
rules:
  required-structure: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# My Plan

## Goal

Describe the goal here.

## Tasks

List tasks here.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# My Plan

## Goal

Describe the goal here.
```

<?/include?>

## Diagnostics

| Condition            | Message                                                                       |
|----------------------|-------------------------------------------------------------------------------|
| section missing      | missing required section "## Settings"                                        |
| wrong level          | heading level mismatch for "Settings": expected h2, got h3                    |
| extra section        | unexpected section "## Extra" (expected "## Settings")                        |
| out of order         | section "## Tasks" out of order: expected after "## Goal"                     |
| heading sync         | heading does not match frontmatter: expected "MDS001" (from id), got "MDS002" |
| body sync            | body does not match frontmatter field "description": expected "..."           |
| front matter schema  | front matter does not satisfy schema CUE constraints: ...                     |
| filename mismatch    | filename "foo.md" does not match required pattern "[0-9]*_*.md"               |
| misplaced require    | <?require?> is only recognized in schema files; this directive has no effect  |
| schema include loop  | cyclic include: a.md -> b.md -> a.md                                          |
| content missing      | missing required content "code-block lang=yaml" inside ## Examples            |
| content unexpected   | unexpected content "table" inside ## Examples (expected "paragraph")          |
| content out of order | content "table" out of order: expected after "code-block lang=yaml"           |
| code block lang      | code block language "json" does not match required "yaml"                     |
| table columns        | table headers [Key Value] do not match required [Setting Default]             |

## Meta-Information

- **ID**: MDS020
- **Name**: `required-structure`
- **Status**: ready
- **Default**: enabled
- **Fixable**: index side-output only (when `schema.index:` is set)
- **Implementation**: [source](./)
- **Guide**:
  [directive guide](../../../docs/guides/directives/enforcing-structure.md)
- **Category**: meta

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
- [Schema field types](../../../docs/reference/schema-types.md)
