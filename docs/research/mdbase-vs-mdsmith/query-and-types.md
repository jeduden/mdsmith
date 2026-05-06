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

## What gets typed

The short version: **mdbase types the front
matter and the filename pattern; mdsmith schemas
type the front matter and the body heading
structure. Neither types the body content as a
structured value.** Both treat `file.body` as a
string for query purposes but neither has a
"this paragraph must have these properties"
type system.

Concretely:

| Aspect typed                                    | mdbase                                          | mdsmith schemas (MDS020)             |
|-------------------------------------------------|-------------------------------------------------|--------------------------------------|
| Front matter field types                        | yes — 12 named field types (spec §7)            | yes — CUE expressions in schema FM   |
| Field-level constraints (min, max, regex)       | yes (per-type knobs)                            | yes (full CUE)                       |
| Required vs optional fields                     | yes (`required: true`)                          | yes (CUE `?` suffix or non-`?`)      |
| Filename pattern                                | yes (`path_pattern: "tasks/{date}-{title}.md"`) | yes (`<?require?>` with `filename`)  |
| Computed FM fields                              | yes (`computed:` block, expression-based)       | no (CUE is structural, not computed) |
| Cross-field constraints                         | limited (per-type knobs)                        | yes (full CUE: `if a then b >= 0`)   |
| Type assignment via FM presence                 | yes (`fields_present`, `where`)                 | no (kinds via explicit tag or glob)  |
| Body heading structure                          | no — body is a `file.body` string               | yes — schema body carries template   |
| Body content patterns (paragraphs, code blocks) | no                                              | no                                   |
| Body cross-references                           | no (link graph is separate)                     | no (cross-file links are MDS027)     |
| Sentence-level prose properties                 | no                                              | no (handled by separate rules)       |
| Cross-file value coherence                      | yes (`unique_values` in Rust impl)              | no                                   |

So the answer to "is mdbase's type system limited
to front matter?" is **almost — but with two
extensions worth naming**:

1. **Filename `path_pattern`.** A type can
   declare a path template like
   `tasks/{date}-{title}.md`. Files claiming
   that type are validated against it. The
   pattern uses FM field values, so it ties FM
   to filesystem layout. Not strictly "in" the
   FM, but derived from it.
2. **Cross-file `unique_values`.** The Rust
   reference impl maintains a `unique_values`
   table for ID uniqueness across the
   collection. A type can declare `id` is
   unique; the validator checks no two files
   share the same ID. The constraint is
   per-field, but it spans files.

Beyond those two, every typed assertion in
mdbase reduces to a property of one file's front
matter (or a derivation of it). The body is
unstructured from the type system's perspective.

mdsmith goes one layer further: a schema's body
*is* a heading template, and MDS020 enforces
that the document's headings match. So mdsmith
types both **the file-as-record** (FM, like
mdbase) and **the file-as-document** (heading
skeleton, unlike mdbase). This is a real
distinction in what the schema authors are
allowed to assert.

Neither tool types body content beyond
structure. Whether a paragraph is well-formed
prose, whether a code block has the right
language tag, whether a list contains a TODO —
these live in mdsmith's *separate rule set* (not
its schemas), and in mdbase they live nowhere
(out of scope for the data layer).

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
| Schema composition                | `extends: base`                           | CUE imports + `<?include?>` in schema body      |
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
mdsmith query 'status: "open" & priority: int & >=3' tasks/
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
- **Composable via CUE imports.** A query can
  import a definition from a `.cue` file and
  reference it (`import "x.com/types"`,
  `task.#OpenHigh`). Rarely used today.
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

| Concern                   | mdsmith CUE                        | mdbase Bases DSL                       |
|---------------------------|------------------------------------|----------------------------------------|
| Surface form              | one expression (struct literal)    | YAML doc + expression strings          |
| Filtering                 | unification (constraint match)     | boolean expression eval                |
| Sort                      | no                                 | `order_by` clause                      |
| Pagination                | no                                 | `limit` / `offset`                     |
| Body access               | no (FM only)                       | `file.body.contains/.matches`          |
| Cross-file traversal      | no                                 | `link.asFile().property` (depth ≤ 10)  |
| Aggregation               | no                                 | `group_by` + `aggregate` + `formulas`  |
| Date arithmetic           | bounds via CUE `>=`/`<=` on string | duration strings (`+ "7d"`)            |
| Type checking of query    | structural unification             | runtime evaluation; type_error per row |
| Composition / imports     | CUE imports                        | none beyond YAML structure             |
| Reusable named queries    | yes (CUE definitions)              | not in the spec                        |
| Error on bad query syntax | parse error before execution       | parse error before execution           |
| Error on type mismatch    | unification failure                | per-row null + type_error diagnostic   |

### Two ways to read the same query

Take **"open tasks at priority 3 or above"**.

```bash
mdsmith query 'status: "open" & priority: int & >=3'
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

**mdsmith: CUE imports + schema include.** A
schema can `<?include?>` another schema's body
fragments to share heading templates. The CUE
in the schema's FM can import shared
definitions:

```cue
// internal/concepts/base.cue
package concepts

#Base: {
    id: =~"^[A-Z]+-[0-9]+$"
    created: =~"^\\d{4}-\\d{2}-\\d{2}"
}
```

```markdown
<!-- tasks/proto.md -->
---
import "github.com/jeduden/mdsmith/concepts"
concepts.#Base
status: '"open" | "done"'
---
<?include
file: ../common/base-template.md
?>

# {title}

## Status notes
```

CUE's unification means the imported
constraints intersect with the child's
constraints — closer to mdbase's "extends"
semantics, but more powerful (the child can
strengthen, not just override).

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
   the project name"). For doc consistency
   this is genuinely useful and not yet
   covered.
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

The first two are clear candidates for either
project. The third is a coordination problem
that needs cross-tool buy-in.

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
