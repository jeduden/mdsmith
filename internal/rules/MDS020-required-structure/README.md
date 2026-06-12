---
id: MDS020
name: required-structure
status: ready
description: Document structure and front matter must match its schema.
category: structural
nature: structure
maintainability:
  signal: files of a kind that lack an explicit section schema
  fix: declare `kinds.<name>.schema` inline or point `kinds.<name>.rules.required-structure.schema` at a proto file
  for-diagnostic: false
markdownlint:
  - id: MD043
    name: required-headings
    partial: false
    default: true
rumdl:
  - id: MD043
    name: required-headings
    partial: false
    default: true
mado: []
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS020: required-structure

Document structure and front matter must match its schema.

## Settings

| Setting         | Type   | Default | Description                                                                                                                |
| --------------- | ------ | ------- | -------------------------------------------------------------------------------------------------------------------------- |
| `schema`        | string | `""`    | Path to a schema file (a `proto.md`)                                                                                       |
| `inline-schema` | map    | (unset) | Inline schema injected by `kinds.<name>.schema:`; not usually written by hand on a rule. DefaultSettings does not list it. |
| `placeholders`  | list   | `[]`    | Placeholder tokens to treat as opaque; see [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md) |

Useful tokens: `cue-frontmatter`.

When neither `schema` nor `inline-schema` is set the rule skips structure
and front matter validation, but still warns on misplaced `<?require?>`
directives. Use overrides or `kinds:` to apply schemas to specific file groups.

A kind may declare its schema in either form. The config loader
rejects a kind that sets both — see [file kinds](../../../docs/guides/file-kinds.md).

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
| ---------- | ------ | ------------------------------------- |
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

### Optional and deprecated fields

Append `?` to mark a key optional (absent allowed,
present still typed). To deprecate, swap the value
for `{type, deprecated: true}` (see file-kinds.md).

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
          sections:
            - heading: "Outcome"
        - heading:
            regex: '.+'
            repeat: { min: 0 }
        - heading: "References"
```

Section keys:

- `heading:` — discriminator. `null` for the preamble
  (content before any heading; valid only as the
  first entry), a string for an exact-match literal
  (regex-escaped into the matcher), or a mapping
  `{ regex, repeat?, sequential? }` for the full
  form. The level for string headings comes from
  depth in the tree (root sections are H2; nested
  sections are H3, then H4, …).
- `sections:` — nested sections one level deeper.
  Rejected on preamble entries (the first heading
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
  Rejected on slot scopes.

The wildcard slot — `{ regex: '.+', repeat: { min: 0 } }` —
absorbs zero or more unlisted sections at its position.
Surrounding listed sections keep their order; out-of-order
detection still claims a heading whose text matches a later
listed scope. Slots reject `sections:`, `rules:`,
`closed:`, and `content:`. The preamble (`heading: null`)
accepts `closed:`, `rules:`, and `content:` for its line
range but rejects `sections:`.

See the
[section-schema reference](../../../docs/reference/section-schema.md)
for the full grammar — `regex:` body, `digits` /
`fmvar(name)` helpers, and `repeat: { min, max }`. H1 is
reserved for `first-line-heading`; inline schemas
constrain H2 and below.

### Cross-references

A `cross-references:` block names text patterns whose
matches must resolve to a real heading. Each entry fills
numeric (`{n}`, `{1}`, …) or named captures from the
regex into `must-match:`; the result is slugified and
looked up in the heading slug set.

```yaml
schema:
  cross-references:
    - pattern: "\\bStep (\\d+)\\b"
      must-match: "Step {n}"
      skip-lines-matching: "^> "
```

`skip-lines-matching:` (regex) exempts blockquoted stale
text and version-history notes from the check.

### Acronyms

An `acronyms:` block flags all-caps tokens (length 2-6,
leading letter, alphanumeric) on first use inside a
configured scope when they appear without a parenthesised
expansion. `known-safe:` lists exempt tokens; `scope:`
restricts the check to matching sections (omit it for
document-wide). First-use state is per-scope.

```yaml
schema:
  acronyms:
    known-safe: [API, HTTP, TLS, JSON]
    scope: ["Check", "Expected"]
```

### Index side-output

An `index:` block asks `mdsmith fix` to write a JSON
side-output next to the source file. `mdsmith check` is
read-only (no write). Output paths are resolved relative
to the document's directory; absolute paths and `..`
traversal are rejected.

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
- `headings` — flat list of `{level, text, slug, line}`

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

Every schema diagnostic names the field, the value, the
constraint, and (when it can) a hint:

```text
status: got "draf", expected one of: "draft", "open", "done"
  (did you mean "draft"?)
  ↳ plan/proto.md:4 — required by schema
```

The `↳` line attaches the schema source as a related location;
editors render it as a `relatedInformation` entry.

| Condition            | Message                                                               |
| -------------------- | --------------------------------------------------------------------- |
| section missing      | `## Settings: got <missing>, expected section to be present`          |
| wrong level          | `Settings: got h3, expected h2`                                       |
| extra section        | `## Extra: got <present>, expected not declared in schema`            |
| out of order         | `## Tasks: got <out of order>, expected in declared order`            |
| heading sync         | `heading does not match frontmatter: expected "X" (from id), …`       |
| body sync            | `body does not match frontmatter field "description": expected …`     |
| front matter schema  | `status: got "draf", expected one of: "draft", "open", "done" …`      |
| filename mismatch    | `filename: got "foo.md", expected filename matching glob [0-9]*_*.md` |
| misplaced require    | `<?require?> is only recognized in schema files; …`                   |
| schema include loop  | `cyclic include: a.md -> b.md -> a.md`                                |
| content missing      | missing required content "code-block lang=yaml" inside ## Examples    |
| content unexpected   | unexpected content "table" inside ## Examples (expected "paragraph")  |
| content out of order | content "table" out of order: expected after "code-block lang=yaml"   |
| code block lang      | code block language "json" does not match required "yaml"             |
| table columns        | table headers [Key Value] do not match required [Setting Default]     |

CUE constraints render in user vocabulary:

| CUE shape            | Rendered as                      |
| -------------------- | -------------------------------- |
| `"a" \| "b" \| "c"`  | `one of: "a", "b", "c"`          |
| `=~"^FOO-[0-9]{4}$"` | `string matching ^FOO-[0-9]{4}$` |
| `int & >=1 & <=5`    | `int between 1 and 5`            |
| `string & != ""`     | `non-empty string`               |
| `bool`               | `true or false`                  |
| anything else        | the raw CUE expression           |

Hints fire on string disjunctions (Levenshtein ≤ 2 of a valid
literal) and integer ranges (nearest bound when just outside).
Other shapes get no hint.

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
- [Schema field types](../../../docs/reference/schema-types.md)

## Meta-Information

- **ID**: MDS020
- **Name**: `required-structure`
- **Status**: ready
- **Default**: enabled
- **Fixable**: index side-output only (when `schema.index:` is set)
- **Implementation**: [source](./)
- **Guide**:
  [directive guide](../../../docs/guides/directives/enforcing-structure.md)
- **Category**: structural
- **markdownlint**: [MD043][mdl-md043] (required-headings)
- **rumdl**: [MD043][rumdl-md043] (required-headings)

[mdl-md043]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md043.md
[rumdl-md043]: https://rumdl.dev/md043/
