---
id: MDS020
name: required-structure
status: ready
description: Document structure and front matter must match its schema.
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
validates document front matter:

```yaml
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
```

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
  this scope produce a diagnostic. Default `false`.
- `rules:` — per-scope rule-config overrides. Each
  entry maps a rule name to a settings map that
  applies on top of the rule's defaults inside the
  scope's range. Today's apply is a plain
  ApplySettings call, not a config-style deep-merge —
  keys the override sets replace the defaults
  wholesale.

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

## Config

Enable with a schema for rule READMEs:

```yaml
overrides:
  - glob: ["internal/rules/*/README.md"]
    rules:
      required-structure:
        schema: internal/rules/proto.md
```

Apply a user-authored schema to all story files via a
`kinds:` declaration:

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

Or set the path inline on an override:

```yaml
overrides:
  - glob: ["stories/**/*.md"]
    rules:
      required-structure:
        schema: schemas/story.md
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

| Condition           | Message                                                                       |
|---------------------|-------------------------------------------------------------------------------|
| section missing     | missing required section "## Settings"                                        |
| wrong level         | heading level mismatch for "Settings": expected h2, got h3                    |
| extra section       | unexpected section "## Extra" (expected "## Settings")                        |
| out of order        | section "## Tasks" out of order: expected after "## Goal"                     |
| heading sync        | heading does not match frontmatter: expected "MDS001" (from id), got "MDS002" |
| body sync           | body does not match frontmatter field "description": expected "..."           |
| front matter schema | front matter does not satisfy schema CUE constraints: ...                     |
| filename mismatch   | filename "foo.md" does not match required pattern "[0-9]*_*.md"               |
| misplaced require   | <?require?> is only recognized in schema files; this directive has no effect  |
| schema include loop | cyclic include: a.md -> b.md -> a.md                                          |

## Meta-Information

- **ID**: MDS020
- **Name**: `required-structure`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Guide**:
  [directive guide](../../../docs/guides/directives/enforcing-structure.md)
- **Category**: meta

## See also

- [Placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
