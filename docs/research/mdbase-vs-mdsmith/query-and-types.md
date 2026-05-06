---
summary: >-
  Deep dive into the query languages and type systems of mdsmith and mdbase
  — what each tool actually types, how schemas and queries compose, where
  the languages diverge in expressiveness, and the answer to whether
  mdbase's type system is limited to front matter.
---
# Query languages and type systems

This doc goes deeper than the catalog tables in
[features.md §5](features.md) (type system) and
[§8](features.md) (query language). The summary
there names the surfaces; this doc walks the
shape of each, side by side, with concrete
examples of what the languages can and cannot
express.

Three questions guide the structure:

1. What does each tool's type system actually
   type? (Front matter? Body? Filenames?
   Computed values?)
2. What do the query languages look like as
   languages — syntax, semantics, composition,
   error handling?
3. Where do the two diverge in expressiveness,
   and where does each break down?

## What gets typed (full enforcement surface)

The honest answer requires looking at **all** the
mechanisms each tool ships, not just the ones
labelled "type system". mdbase has a single
mechanism (the type system in `_types/`).
mdsmith has two: MDS020 schemas (the closest
direct analogue to mdbase types) **and** a
broader rule set of 54 lint rules, many of which
encode structural constraints on the document.
Treating the rules as out-of-scope would
underrate mdsmith's actual coverage.

So the framing here is: **what does each tool
collectively enforce about a document?** The
mechanism (typed schema vs lint rule) matters
less than the shape of the enforcement.

### Full enforcement matrix

Three columns per group: what mdbase types via
its schema; what mdsmith's MDS020 schema types;
what mdsmith's broader rule set enforces. Cells
are evaluated as "the user can declare this
constraint and have it enforced". Tables are
split by category for readability.

#### Front matter and type assignment

| Aspect                           | mdbase types             | mdsmith MDS020 schema          | mdsmith broader rule set                            |
|----------------------------------|--------------------------|--------------------------------|-----------------------------------------------------|
| FM field types                   | yes (12 named types, §7) | yes (CUE in schema FM)         | n/a                                                 |
| FM field constraints             | per-type knobs           | full CUE                       | n/a                                                 |
| FM required / optional           | `required: true`         | CUE `?` suffix                 | n/a                                                 |
| FM cross-field constraints       | limited                  | full CUE (`if/then`)           | n/a                                                 |
| Computed FM fields               | yes (`computed:` block)  | no (CUE is structural)         | n/a                                                 |
| Generated FM values              | yes (ULID, timestamps)   | n/a (mdsmith doesn't write)    | n/a                                                 |
| Type assignment by FM presence   | yes (`fields_present`)   | no                             | no (kinds: globs / tags)                            |
| Type assignment by FM where      | yes (`where:`)           | no                             | no                                                  |
| FM ↔ body sync (e.g. title ↔ H1) | no                       | yes (placeholders in template) | yes (catalog/include/build directives drift-detect) |

#### Filename, directory, headings

| Aspect                    | mdbase types   | mdsmith MDS020 schema       | mdsmith broader rule set                  |
|---------------------------|----------------|-----------------------------|-------------------------------------------|
| Filename pattern          | `path_pattern` | `<?require?>` filename glob | MDS033 directory-structure                |
| First-heading rule        | no             | template's first H1         | MDS004 first-line-heading                 |
| Heading level increments  | no             | template implies it         | MDS003 heading-increment                  |
| Heading uniqueness        | no             | no                          | MDS005 no-duplicate-headings              |
| Single H1 per file        | no             | template's H1               | MDS051 single-h1                          |
| Heading punctuation       | no             | no                          | MDS017 no-trailing-punctuation-in-heading |
| Emphasis used as heading  | no             | no                          | MDS018 no-emphasis-as-heading             |
| Required heading sequence | no             | yes (template body)         | n/a (this *is* MDS020)                    |

#### Section and file caps; prose

| Aspect                         | mdbase types | mdsmith MDS020 schema | mdsmith broader rule set                  |
|--------------------------------|--------------|-----------------------|-------------------------------------------|
| Section emptiness              | no           | no                    | MDS030 empty-section-body                 |
| Section length cap             | no           | no                    | MDS036 max-section-length                 |
| File length cap                | no           | no                    | MDS022 max-file-length                    |
| Token budget (LLM context)     | no           | no                    | MDS028 token-budget                       |
| Paragraph readability (ARI)    | no           | no                    | MDS023 paragraph-readability              |
| Paragraph structure            | no           | no                    | MDS024 paragraph-structure                |
| Conciseness                    | no           | no                    | MDS029 conciseness-scoring (experimental) |
| Cross-file content duplication | no           | no                    | MDS037 duplicated-content                 |

#### Tables, code, lists

| Aspect                         | mdbase types | mdsmith MDS020 schema | mdsmith broader rule set             |
|--------------------------------|--------------|-----------------------|--------------------------------------|
| Table format                   | no           | no                    | MDS025 table-format                  |
| Table readability              | no           | no                    | MDS026 table-readability             |
| Fenced code style              | no           | no                    | MDS010 fenced-code-style             |
| Fenced code language tag       | no           | no                    | MDS011 fenced-code-language          |
| Blank lines around fenced code | no           | no                    | MDS015 blank-line-around-fenced-code |
| Unclosed code block            | no           | no                    | MDS031 unclosed-code-block           |
| Spaces in code spans           | no           | no                    | MDS052 no-space-in-code-spans        |
| List marker style              | no           | no                    | MDS045 list-marker-style             |
| Ordered list numbering         | no           | no                    | MDS046 ordered-list-numbering        |
| List indent                    | no           | no                    | MDS016 list-indent                   |

#### Links, references, cross-file

| Aspect                      | mdbase types                       | mdsmith MDS020 schema | mdsmith broader rule set              |
|-----------------------------|------------------------------------|-----------------------|---------------------------------------|
| Bare URLs                   | no                                 | no                    | MDS012 no-bare-urls                   |
| Cross-file link integrity   | yes (L4 + `validate_exists`)       | no                    | MDS027 cross-file-reference-integrity |
| Cross-file rename safety    | yes (L5 rewrites refs)             | no                    | no (MDS027 detects, doesn't fix)      |
| Wikilink resolution         | yes (L4 — `[[Page]]`)              | no                    | no (L-1 candidate)                    |
| ID-based link resolution    | yes (`id_field` per spec)          | no                    | no (L-2 candidate)                    |
| Cross-file value uniqueness | yes (`unique` per type, Rust impl) | no                    | no                                    |
| Reference-style link policy | no                                 | no                    | MDS043 no-reference-style             |
| Unused link reference defs  | no                                 | no                    | MDS053 no-unused-link-definitions     |
| Undefined reference labels  | no                                 | no                    | MDS054 no-undefined-reference-labels  |
| Spaces in link text         | no                                 | no                    | MDS049 no-space-in-link-text          |

#### Style, formatting, generated content

| Aspect                           | mdbase types | mdsmith MDS020 schema | mdsmith broader rule set      |
|----------------------------------|--------------|-----------------------|-------------------------------|
| Image alt text                   | no           | no                    | MDS032 no-empty-alt-text      |
| Inline HTML policy               | no           | no                    | MDS041 no-inline-html         |
| Emphasis style                   | no           | no                    | MDS042 emphasis-style         |
| Horizontal rule style            | no           | no                    | MDS044 horizontal-rule-style  |
| Ambiguous emphasis               | no           | no                    | MDS047 ambiguous-emphasis     |
| Proper-name capitalization       | no           | no                    | MDS050 proper-names           |
| Markdown flavor (CommonMark/GFM) | no           | no                    | MDS034 markdown-flavor        |
| Trailing whitespace / tabs       | no           | no                    | MDS006–009 whitespace rules   |
| Line length                      | no           | no                    | MDS001 line-length            |
| Blank lines around blocks        | no           | no                    | MDS013–015                    |
| Generated section drift          | no           | no                    | MDS019/021/038/039 directives |
| Build recipe safety              | no           | no                    | MDS040 recipe-safety          |

The table is one-sided in row count — that's
the point. mdsmith's enforcement surface is
**broad and shallow** (54 rules covering many
document properties). mdbase's is **narrow and
deep** (one type system, but it goes deep on
field-level constraints, generated values,
cross-file uniqueness, and link integrity).

### So is mdbase's type system limited to FM?

Yes, with two named extensions:

1. **Filename `path_pattern`.** A type can
   declare `path_pattern: "tasks/{date}-{title}.md"`,
   tying FM field values to filesystem layout.
2. **Cross-file `unique_values`.** The Rust
   reference impl maintains a per-collection
   uniqueness table; types can declare `unique`
   on an `id` field.

Beyond those, every typed assertion in mdbase
reduces to a property of one file's FM (or a
derivation of it via computed fields, or a
property of the link graph the FM declares).
The body content is unstructured from the type
system's perspective. `file.body` is queryable
as a string but not typed.

### So what about mdsmith's "type system"?

mdsmith does not have one type system; it has
**MDS020 schemas plus a 54-rule constraint
library**. Two distinct surfaces:

1. **MDS020 schema** (`proto.md` per kind):
   types FM via CUE expressions; types body
   heading structure via the template; types
   filename via `<?require?>`.
2. **The broader rule set**: imposes structural
   constraints on headings (level increments,
   uniqueness, single-H1), paragraphs
   (readability, structure, conciseness), code
   blocks, tables, links, lists, file length,
   token budget, directory layout, prose
   style, and more — all without a per-file
   schema declaration. Rules apply by default
   or per-kind override.

The two surfaces compose. A schema can declare
"this file's FM has fields X and Y, and its body
has these headings"; the surrounding rule set
adds "and the prose is readable, the tables fit,
the headings increment, no link is broken".

This is a real architectural difference: mdbase
puts everything in the type, mdsmith splits
between per-kind schemas and per-rule
enforcement. Either approach can express most
constraints; the language for declaring them
differs. mdbase's vocabulary is "type fields";
mdsmith's is "rules with settings".

### Where each is genuinely silent

Both tools share a few gaps:

- **Body content patterns beyond headings.** Whether
  paragraph N is a definition, whether a list
  contains a TODO, whether a code block is
  Python — neither tool ships a way to declare
  this.
- **Schema-aware query validation.** A typo in
  a field name returns no results rather than
  "no such field in type X" at parse time.
- **Cross-tool schema portability.** mdsmith
  CUE and mdbase DSL describe overlapping
  shapes; nothing translates between them.

mdsmith's experimental MDS029
(conciseness-scoring) is a step toward
content-quality typing but is opt-in and
classifier-based rather than declarative.

## Front matter and body — keeping them in sync

Front matter and body are structurally disjoint
pieces of a Markdown file. The YAML at the top
parses to one tree, the Markdown below to another;
nothing in standard Markdown makes them
co-vary. A file with `title: Migration Plan` and
a body H1 reading `# Outline` is syntactically
fine — but obviously incoherent. What does each
tool offer to **enforce coherence between front
matter and body**?

This is a place mdsmith is materially stronger
than mdbase, and it falls out of the broader
rule-set/directive split:

### mdsmith mechanisms for FM↔body sync

Five mechanisms tie FM to body content, all
schema-style or directive-driven:

| Mechanism                     | What it ties                                                                                                                   | Rule                        |
|-------------------------------|--------------------------------------------------------------------------------------------------------------------------------|-----------------------------|
| Heading template placeholders | `# {title}` in schema body → body H1 must match FM `title`                                                                     | MDS020 (required-structure) |
| Catalog directive             | `<?catalog?>` table cells reference FM fields like `{summary}`, `{filename}` of matched files                                  | MDS019 (catalog)            |
| Include directive variables   | `<?include?>` body can substitute FM fields from the host file                                                                 | MDS021 (include)            |
| Build directive params        | `<?build?>` recipe params can come from FM                                                                                     | MDS039 (build)              |
| TOC directive                 | `<?toc?>` derives body section list from body headings (not from FM, but keeps a body-derived index in sync with body content) | MDS038 (toc)                |

The first four detect drift: edit the FM, the
body table or heading is now stale; `mdsmith fix`
regenerates it. The fifth tracks body-internal
sync (heading list ↔ TOC).

Concretely, an MDS020 schema body of the form

```markdown
---
title: 'string'
---
# {title}

## Description
```

requires that any document tagged with this kind
has a body H1 matching its FM `title` exactly
(after placeholder substitution). Change the FM
title without touching the body, and the lint
fires on the next run.

### mdbase mechanisms for FM↔body sync

mdbase has one mechanism, and it ties FM to the
**filesystem**, not the body:

| Mechanism      | What it ties                      | Where   |
|----------------|-----------------------------------|---------|
| `path_pattern` | FM field values → filename / path | spec §5 |

A type like `tasks/{date}-{title}.md` enforces
that an FM `date: 2026-05-04, title: oidc` file
lives at `tasks/2026-05-04-oidc.md`. Useful, but
about filesystem layout.

For the body itself, mdbase offers nothing.
`file.body` is queryable as a string; queries
can compare FM fields against body substrings
(`file.body.contains(title)`) at read time, but
there is no enforcement: writing a body that
doesn't match FM is permitted by mdbase. The
spec does not define body templates, body
heading constraints, or body-from-FM generation.

### Side-by-side

| Sync direction           | mdsmith                                   | mdbase         |
|--------------------------|-------------------------------------------|----------------|
| FM → filename            | n/a (MDS033 directory-structure separate) | `path_pattern` |
| FM → body H1             | MDS020 heading template with `{title}`    | no             |
| FM → body table cells    | `<?catalog?>` directive (drift-detected)  | no             |
| FM → body include vars   | `<?include?>` directive variables         | no             |
| FM → body build artifact | `<?build?>` directive                     | no             |
| Body → body TOC          | `<?toc?>` directive                       | no             |
| FM → query-only check    | (could grow; not present)                 | yes, read-only |

### What this implies

An mdbase team using FM as the typed source of
truth has no built-in way to ensure their body
H1 matches their FM title, that a catalog page
reflects current FM, or that referenced FM
fields appear in the body. They handle this
with conventions or external tools.

An mdsmith team has these out of the box. The
five directives cover most FM→body relations
that come up in practice; MDS020 placeholders
are the last-mile linker for the schema-body
case.

This is not a gap mdbase is silent on by
oversight — the tool is scoped to data-layer
typing. Body content is treated as opaque
prose. mdsmith covers it because the broader
rule-and-directive surface includes generated
content and template-driven validation.

For the question "what does each tool offer to
keep FM and body in sync?", the honest answer
is: **mdsmith ships explicit FM↔body sync
mechanisms across schemas and directives;
mdbase offers FM↔filename via `path_pattern`
and nothing on the body side.** A team that
needs both is running mdsmith for the body sync
even if mdbase owns the FM types.

## How the type definitions look

A worked example: a `task` type with five fields
(`id`, `title`, `status`, `priority`,
`assignee`), filename pattern, and the
constraint that `priority` is between 1 and 5.

### mdbase: `_types/task.md`

```markdown
---
name: task
extends: base
strict: warn
path_pattern: tasks/{date}-{title}.md
fields:
  id:
    type: string
    pattern: "^TASK-[0-9]{4}$"
    required: true
    unique: true
    generated: ulid
  title:
    type: string
    required: true
    min_length: 5
  status:
    type: enum
    values: [open, in-progress, done]
    required: true
    default: open
  priority:
    type: integer
    min: 1
    max: 5
    default: 3
  assignee:
    type: link
    target: person
    validate_exists: true
computed:
  days_overdue:
    type: integer
    expr: (today() - due) / 86400000
---
# Task

A unit of work with a status, priority, and
assignee.
```

The type's *schema* is the front matter; the
body is human-facing documentation. Computed
fields (`days_overdue`) live in a dedicated
block and are evaluated at read time.

### mdsmith: `tasks/proto.md`

```markdown
---
id: '=~"^TASK-[0-9]{4}$"'
title: 'string & strings.MinRunes(5)'
status: '"open" | "in-progress" | "done"'
priority: 'int & >=1 & <=5'
assignee: 'string'
"due?": 'string & =~"^\\d{4}-\\d{2}-\\d{2}$"'
---
<?require
filename: "TASK-[0-9]+.md"
?>

# {title}

## Description

## Acceptance criteria

## Status notes
```

The type's *FM constraints* are CUE expressions
in the schema's front matter. The type's
*structure constraints* are the heading template
in the schema's body. The `<?require?>` PI
declares filename rules. CUE handles regex,
disjunctions, ranges, and optional fields
directly.

### What each can and cannot express

| Constraint                        | mdbase DSL                                | mdsmith CUE                                     |
|-----------------------------------|-------------------------------------------|-------------------------------------------------|
| Regex on string                   | `pattern: "^TASK-[0-9]{4}$"`              | `=~"^TASK-[0-9]{4}$"`                           |
| Numeric range                     | `min: 1, max: 5`                          | `int & >=1 & <=5`                               |
| Enum                              | `values: [open, …]`                       | `"open" \| "in-progress" \| "done"`             |
| Optional field                    | omit `required: true`                     | append `?` to the key (`"due?"`)                |
| Default value                     | `default: open`                           | not directly; CUE has `*"open" \| string`       |
| Generated value (ULID, timestamp) | `generated: ulid`                         | not in the schema (mdsmith doesn't write FM)    |
| Unique across collection          | `unique: true`                            | not directly                                    |
| Cross-field implication           | not in DSL                                | `if status == "done" then completed_at != null` |
| Computed at read time             | `computed: { expr: ... }`                 | not directly (CUE is structural)                |
| Filename template                 | `path_pattern: "tasks/{date}-{title}.md"` | `<?require?>` with regex / glob                 |
| Body heading template             | not in scope                              | schema body                                     |
| Schema composition                | `extends: base`                           | `<?include?>` in schema body (no CUE imports)   |
| Inheritance overrides             | child fields fully replace parent's       | CUE unification (intersection)                  |

CUE wins on cross-field constraints, structural
inheritance, and rich pattern composition.
mdbase wins on default values, generated values
(ULID, timestamps), and computed fields. Most of
the gap on each side is closable in principle —
mdsmith could grow defaults and generated
fields (S-5, S-6 in
[learn-from-mdbase.md](learn-from-mdbase.md));
mdbase's spec §11 expression language could
grow to express more cross-field constraints.

## When types fire

Type checking happens at three different moments
across the two tools, with implications for how
errors surface.

| Moment                      | mdbase                                            | mdsmith                                        |
|-----------------------------|---------------------------------------------------|------------------------------------------------|
| File creation (`create`)    | yes — validates before persisting                 | n/a (mdsmith doesn't author files)             |
| File update (CRUD `update`) | yes — validates merged result                     | n/a                                            |
| Lint pass (`mdsmith check`) | n/a (mdbase doesn't lint)                         | yes — MDS020 validates against schema          |
| Query against a typed field | yes — type errors return null + emit `type_error` | indirectly (CUE unification fails on mismatch) |
| Editor save (LSP)           | mdbase-lsp surfaces validation errors             | mdsmith LSP surfaces MDS020 diagnostics        |
| Migration / backfill        | yes (L6 migration manifests apply transforms)     | n/a                                            |

The asymmetry is that **mdbase types fire on
write**; **mdsmith schemas fire on lint**. The
mdbase model gives earlier feedback (the file
is rejected before bad FM ever lands on disk).
The mdsmith model is less invasive (any text
editor can write whatever; the lint pass tells
you later) but blocks at PR time via CI.

For an agent loop the difference matters: an
agent writing files via `mdbase create` learns
of type errors at the moment of creation. An
agent writing files via raw editor + `mdsmith
check` learns at the next lint pass. Neither
strictly better; the agent design picks the
trade-off.

## Query languages, side by side

The features.md catalog lists operators and
methods. This section walks **what kind of
language each is** and where the practical
limits sit.

### mdsmith query: CUE struct literal

`mdsmith query EXPR FILES` accepts a CUE struct
literal. The engine wraps it in `{...}` and
unifies it against each file's effective front
matter. Unification succeeds when the file's
data is a *subtype* of the query — every
constraint in the query must be satisfiable
against the file's FM.

```bash
mdsmith query 'status: "open", priority: int & >=3' tasks/
```

- **Lexically a CUE expression.** Comments,
  imports, definitions all available (rarely
  used in practice; queries are usually
  one-line struct literals).
- **Semantically a constraint, not a script.**
  No iteration, no aggregation, no `for`. The
  result is "every file whose FM passes the
  constraint" — a set membership test, not a
  computation.
- **Type-aware.** Constraints like `int`,
  `string`, `[…]` operate on CUE types, so a
  field that is supposed to be an int is
  matched against an int constraint and fails
  cleanly if the FM has the wrong type.
- **Not composable across files today.** The
  CLI argument is wrapped in `{...}` and
  compiled as an expression body
  (`internal/query/query.go`), so CUE
  `import "..."` and `package` declarations are
  not accepted at this surface. Reuse happens at
  the shell level — variables, scripts.
- **Limited to filtering.** No sort, limit,
  body access, traversal, or aggregation. The
  command emits matching paths to stdout.

### mdbase query: Bases-compatible expression

`mdbase query` takes a query *document* (YAML
struct with `types`, `where`, `order_by`, etc.).
The `where` value is a string expression in the
DSL described in spec §11.

```yaml
types: [task]
where: status == "open" && priority >= 3
order_by:
  - field: due
    direction: asc
limit: 20
```

- **Lexically a small expression DSL.**
  Operators (`==`, `<`, `&&`), method calls
  (`.contains`, `.matches`), function calls
  (`today()`, `if(...)`).
- **Semantically an evaluation.** The expression
  is evaluated *for each file* against its
  effective FM (and link graph, body string,
  computed fields). Evaluation produces a
  boolean for `where`, a value for `order_by`,
  a count for aggregations.
- **Type-flexible.** Type errors on individual
  values return null and emit a `type_error`
  diagnostic but do not abort the query
  (spec §11). Compare to CUE which fails the
  unification entirely.
- **Composable via the surrounding YAML.**
  `types`, `folder`, `where`, `order_by`,
  `limit`, `offset`, `group_by`, `aggregate`,
  `formulas`, `summaries`, `top_per_group`.
- **Covers more than filtering.** Sort,
  pagination, body search, cross-file
  traversal, aggregation, formulas — all
  inside the DSL.

### Side by side as languages

| Concern                   | mdsmith CUE                                   | mdbase Bases DSL                       |
|---------------------------|-----------------------------------------------|----------------------------------------|
| Surface form              | one expression (struct literal)               | YAML doc + expression strings          |
| Filtering                 | unification (constraint match)                | boolean expression eval                |
| Sort                      | no                                            | `order_by` clause                      |
| Pagination                | no                                            | `limit` / `offset`                     |
| Body access               | no (FM only)                                  | `file.body.contains/.matches`          |
| Cross-file traversal      | no                                            | `link.asFile().property` (depth ≤ 10)  |
| Aggregation               | no                                            | `group_by` + `aggregate` + `formulas`  |
| Date arithmetic           | bounds via CUE `>=`/`<=` on string            | duration strings (`+ "7d"`)            |
| Type checking of query    | structural unification                        | runtime evaluation; type_error per row |
| Composition / imports     | none at the CLI surface (no `import` allowed) | none beyond YAML structure             |
| Reusable named queries    | shell-level only (variables, scripts)         | not in the spec                        |
| Error on bad query syntax | parse error before execution                  | parse error before execution           |
| Error on type mismatch    | unification failure                           | per-row null + type_error diagnostic   |

### Two ways to read the same query

Take **"open tasks at priority 3 or above"**.

```bash
mdsmith query 'status: "open", priority: int & >=3'
```

The mdsmith form says: the file's FM must
unify with the struct
`{status: "open", priority: int & >=3}`. This
is a **constraint match**: any file whose FM
already has `status` and `priority` with those
properties is in the result.

```yaml
where: status == "open" && priority >= 3
```

The mdbase form says: for each file, evaluate
this boolean expression in a context where
`status` and `priority` are the file's FM
values. Files where the expression is true are
in the result. This is **expression
evaluation**.

Both reach the same set of files for this
query. The languages differ in what they
encourage: CUE encourages thinking about FM as
a typed shape; Bases encourages thinking about
FM as named values you compute over.

## Type-aware queries

A subtle point: do the queries actually know the
schema of the data they're filtering?

**mdsmith CUE query.** Yes, structurally.
`priority: int & >=3` enforces that the FM's
`priority` is an int (or the query fails to
match). But the query does *not* consult the
mdsmith schema — it doesn't know there's a
schema declaring `priority: int & >=1 & <=5`.
You can write `priority: int & >=99` and run
it; no schema-level validation happens, you
just get an empty result.

**mdbase query.** Per spec §11 the query
consults the type system insofar as it knows
field types (so `priority >= 3` errors with
`type_error` if a file has `priority: "high"`).
But "consults the schema" in the strong sense —
"refuse to compile a query that references a
field the schema doesn't declare" — isn't part
of the spec. The TS impl doesn't do this; the
Rust impl reads `effective_json` (FM with
defaults) but evaluates the expression
generically.

**What's missing in both.** Neither tool
does **schema-aware query validation**: a query
that references `prirority` (typo) just returns
no results, rather than reporting "no such
field in type `task`". This is a small but
real gap; closing it would surface query bugs
earlier. mdbase's schema is rich enough to
support it; mdsmith CUE could grow it via
schema imports.

## Composition

How types and queries compose differs sharply.

### Type composition

**mdbase: `extends`.** A type can extend
another. Child fields fully replace parent
fields with the same name. Single inheritance,
no diamond. Worked example: `task` extends
`base`, inheriting `id`, `created_at`,
`modified_at`.

```yaml
# _types/base.md
---
name: base
fields:
  id: { type: string, required: true, unique: true }
  created_at: { type: datetime, generated: now_on_create }
  modified_at: { type: datetime, generated: now_on_write }
---

# _types/task.md
---
name: task
extends: base
fields:
  status: { type: enum, values: [open, done] }
---
```

**mdsmith: schema body include + per-field CUE.**
mdsmith schemas don't compose via CUE imports.
The schema's FM is parsed as YAML and converted
into a closed CUE struct (`deriveFrontMatterCUE`
in the MDS020 implementation), so each field's
constraint is the YAML value (a CUE expression
encoded as a string). There's no `import "..."`
in the schema's FM.

What does compose:

- **Body templates via `<?include?>`.** A
  schema's body can splice in fragments from
  other schema bodies, sharing heading
  templates across kinds.
- **Field-level CUE expressions.** Each field's
  constraint is a CUE string; constraints can
  reference CUE built-ins (`strings.MinRunes`,
  regex `=~`, disjunctions `|`, etc.) inline.
  Reuse across schemas means copy-pasting the
  expression string today.

```markdown
<!-- common/base-template.md (a schema-body fragment) -->
# {title}

## Description

## Status notes
```

```markdown
<!-- tasks/proto.md -->
---
id: '=~"^[A-Z]+-[0-9]+$"'
created: '=~"^\\d{4}-\\d{2}-\\d{2}"'
status: '"open" | "done"'
---
<?include
file: ../common/base-template.md
?>
```

The `<?include?>` is structural composition;
the FM constraints are independent per-field
CUE strings. A future plan could add CUE
imports to schemas (S-3 in
[learn-from-mdbase.md](learn-from-mdbase.md)
sketches the inheritance angle), but it is
not in mdsmith today.

### Query composition

Neither tool ships a "named, reusable query"
abstraction. mdbase queries are documents;
mdsmith queries are command-line strings. Both
projects could grow query libraries; neither
has prioritized it.

## Migration between syntaxes

For a team running both tools (the
[interop.md](interop.md) territory), it's worth
knowing which queries translate between
syntaxes and which don't.

| mdbase Bases query                   | Equivalent mdsmith CUE                                | Notes                                             |
|--------------------------------------|-------------------------------------------------------|---------------------------------------------------|
| `status == "open"`                   | `status: "open"`                                      | trivial                                           |
| `priority >= 3`                      | `priority: int & >=3`                                 | trivial                                           |
| `tags.contains("urgent")`            | `tags: [..., "urgent", ...]`                          | CUE syntax for list-contains is awkward but works |
| `status == "open" && priority >= 3`  | `status: "open", priority: int & >=3`                 | trivial                                           |
| `due <= today() + "7d"`              | external date substitution                            | Q-4 in learn-from-mdbase.md                       |
| `assignee.asFile().role == "senior"` | not expressible                                       | L-5 in learn-from-mdbase.md                       |
| `file.body.contains("OIDC")`         | not in `query`; needs native `--body-contains`        | Q-3 in learn-from-mdbase.md                       |
| `group_by: status, aggregate: count` | not in `query`; needs native group / aggregate        | Q-5 in learn-from-mdbase.md                       |
| `order_by: due, limit: 20`           | not in `query`; needs native `--order-by` / `--limit` | Q-1 / Q-2 in learn-from-mdbase.md                 |

The takeaway: **mdsmith CUE handles the filter
core well; mdbase handles everything else.** A
schema-aware bridge that translates the common
filter forms in both directions is feasible
(see interop.md §7 schema bridge sketch); a
bridge that handles aggregation and traversal
would need to compile to native operations on
both sides.

## What's missing in both

Looking across the two systems, three
capabilities are absent everywhere and would
benefit either side.

1. **Structural body typing.** Neither tool
   types "body must have an H1, then 2+ H2s,
   each followed by a paragraph". mdsmith
   schemas come closest with the heading
   template, but they don't reach inside
   sections (no "first paragraph must mention
   the project name", no "this field has a
   word-count cap", no cross-reference
   resolution between body sections). For
   structured Markdown documents — decks,
   runbooks, training material, regulatory
   text — this is the largest untyped surface.
   **Candidate plan: S-7** in
   [learn-from-mdbase.md](learn-from-mdbase.md)
   sketches a rich-body-schema language with
   nested sections, per-field rules
   (word/char counts, forbidden patterns,
   skip rules), cross-reference validation,
   acronym tracking, and index generation.
2. **Schema-aware query validation.** A typo
   in a field name silently returns no
   results. Either tool's schema layer is rich
   enough to power "no such field" errors at
   query parse time, but neither does it.
3. **Cross-implementation schema portability.**
   mdsmith CUE and mdbase DSL describe
   overlapping shapes; nothing converts
   between them automatically.
   [interop.md §7](interop.md) sketches a
   bridge that doesn't exist yet.

S-7 is the most ambitious of the three. The
other two are clear candidates for either
project; the cross-impl bridge is a
coordination problem that needs cross-tool
buy-in.

## Sources

- mdbase spec §5 (types), §6 (matching),
  §7 (field types), §10 (querying), §11
  (expressions): <https://github.com/callumalpass/mdbase-spec>
- TS reference impl source:
  <https://github.com/callumalpass/mdbase>
- Rust reference impl source:
  <https://github.com/callumalpass/mdbase-rs>
- mdsmith MDS020 README:
  `internal/rules/MDS020-required-structure/README.md`
- mdsmith query implementation:
  `internal/query/query.go`
- CUE language reference:
  <https://cuelang.org/docs/references/>
