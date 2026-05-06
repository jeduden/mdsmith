---
summary: >-
  Feature-by-feature comparison of mdsmith and mdbase across
  distribution, configuration, type system, validation, queries, links,
  generated content, prose linting, conventions, cache, CLI, LSP, output
  formats, and security posture.
---
# Feature-by-feature comparison

This document walks the surface of each tool side by
side. Each section names what the feature is, how
mdsmith does it (with code citations), how mdbase
defines it (with spec section refs), and where the
overlap or gap is. Conformance level (`L1`–`L6`) refers
to mdbase's six tiers in spec section 14.

## 1. Distribution and runtime

| Aspect       | mdsmith                               | mdbase                                                               |
|--------------|---------------------------------------|----------------------------------------------------------------------|
| Artifact     | One Go binary (`mdsmith`)             | Spec + reference impl (`mdbase`, TS) + CLI + Rust LSP                |
| Install      | `go install` or download release      | `npm install`, or build LSP via `cargo build --release`              |
| Runtime      | None (static binary)                  | Node 22+ for TS impl and CLI; standalone for LSP                     |
| Plugin model | None — rules baked in at compile time | None at the spec level — but each impl can add tools/extensions      |
| Network      | None at any phase                     | None — files-as-truth principle (spec §0)                            |
| Spec layer   | n/a — single implementation           | Specification document at version 0.2.1                              |
| Conformance  | n/a                                   | Implementations declare a level: e.g. `Conformance: Level 4 (Links)` |

mdbase trades a single canonical tool for an open
standard with multiple conforming implementations.
Today only one implementation suite exists, but the
spec is structured so a second-party Go or Python
impl could replace it without users changing their
files. mdsmith makes the opposite bet: one binary
owns all behavior.

## 2. Configuration

### mdsmith: `.mdsmith.yml`

Top-level YAML at the project root. Optional. Layered
config resolves per file (see CLAUDE.md and
`internal/config/load.go`):

```text
defaults
  ↓ deep-merge
convention preset (portable | github | plain)
  ↓ deep-merge
top-level rules:
  ↓ deep-merge
kinds assigned to file (front-matter or kind-assignment)
  ↓ deep-merge
overrides: [{glob, rules}, …]
```

(Layer order matches `internal/config/merge.go`'s
`effectiveRules`: defaults → convention → user
rules → kinds → overrides, with later layers
winning on conflict.)

Deep-merge semantics (see
`internal/config/deepmerge.go`):

- Maps recurse key-by-key
- Scalars are replaced wholesale
- Lists replace by default; rules opt into append via
  the `rule.ListMerger` interface (e.g.,
  `placeholders:`, `proper-names.names:`)
- A bool-only layer (`rule-name: false`) toggles
  `enabled` without erasing inherited settings

### mdbase: `mdbase.yaml`

Single config file at the collection root. Required:
`spec_version: "0.2.1"`. All other settings optional
(spec §4):

| Field                | Default                           | Purpose                                |
|----------------------|-----------------------------------|----------------------------------------|
| `name`               | —                                 | Human-readable label                   |
| `description`        | —                                 | Free-form purpose                      |
| `extensions`         | `[]`                              | Extra Markdown extensions beyond `.md` |
| `exclude`            | `.git`, `node_modules`, `.mdbase` | Paths or globs to skip                 |
| `include_subfolders` | `true`                            | Recurse into directories               |
| `types_folder`       | `_types`                          | Where type files live                  |
| `explicit_type_keys` | `["type", "types"]`               | Front-matter keys for explicit type    |
| `migrations_folder`  | `<types>/_migrations`             | Migration manifests (L6)               |
| `default_validation` | `warn`                            | Off / warn / error                     |
| `default_strict`     | `false`                           | Reject unknown front-matter fields     |
| `write_nulls`        | `omit`                            | How nulls persist on write             |
| `write_defaults`     | `true`                            | Materialize default-filled fields      |
| `write_empty_lists`  | `true`                            | Persist `[]` rather than omitting      |
| `timezone`           | system                            | IANA name for date functions           |
| `id_field`           | `id`                              | Primary key for link resolution        |
| `rename_update_refs` | `true`                            | Rewrite incoming links on rename       |
| `cache_folder`       | `.mdbase`                         | SQLite cache location                  |

### Where they overlap

| Concern                | mdsmith equivalent             | mdbase equivalent           |
|------------------------|--------------------------------|-----------------------------|
| Project root marker    | `.mdsmith.yml` (optional)      | `mdbase.yaml` (required)    |
| Ignore patterns        | `ignore:` glob list            | `exclude:` glob list        |
| Per-path overrides     | `overrides: [{glob, rules}]`   | per-type `path_glob`        |
| Per-role rule bundle   | `kinds:` map                   | type files in `_types/`     |
| Schema validation flag | per-rule severity (Error/Warn) | `default_validation` global |

Both tools declare schemas as Markdown files. The
difference is where the file lives and what it
covers:

- mdbase types live in a fixed `_types/` folder
  next to the content, discoverable from the file
  tree alone. The front matter carries the typed
  schema; the body is documentation.
- mdsmith schemas (typically `proto.md`) live
  wherever the team puts them, referenced from a
  kind in `.mdsmith.yml` via
  `required-structure.schema:`. The front matter
  carries CUE expressions; the body carries the
  required heading template. mdsmith schemas
  validate body structure as well as front matter.

## 3. Type / kind definitions

### mdsmith: file kinds

Defined in `.mdsmith.yml` under the `kinds:` map. A
kind is a named bundle of rule overrides plus
optionally a `required-structure.schema:` pointer
to a Markdown schema file (`proto.md`) and category
toggles (`internal/config/config.go`):

```yaml
kinds:
  rule-readme:
    rules:
      required-structure:
        schema: internal/rules/proto.md
  api-doc:
    rules:
      line-length:
        max: 120
      token-budget:
        max: 4000
```

Assignment to files works in two ways:

1. Front-matter declaration — `kinds: [rule-readme]`
   or `kind: rule-readme` in the file's YAML
2. Glob from `kind-assignment:` in `.mdsmith.yml`

Inheritance: there is no kind-to-kind inheritance.
A file can carry multiple kinds; each is deep-merged
into the effective config in declaration order.

(Note: `internal/concepts/` in this repo holds
implementation architecture documentation
— placeholder grammar, concept notes — not
user-facing schemas. Schemas are the `proto.md`
files referenced from kinds.)

### mdbase: types

Defined as Markdown files in `_types/`, e.g.
`_types/task.md` (spec §5). The front matter declares
the schema; the body documents the type for humans
who open the vault:

```markdown
---
name: task
extends: base
strict: warn
path_pattern: tasks/{date}-{title}.md
fields:
  status:
    type: enum
    values: [open, in-progress, done]
    required: true
  priority:
    type: integer
    min: 1
    max: 5
    default: 3
  due:
    type: date
  assignee:
    type: link
    target: person
    validate_exists: true
  tags:
    type: list
    items:
      type: string
computed:
  days_overdue:
    type: integer
    expr: (today() - due) / 86400000
---
# Task

A unit of work with a status, priority, and assignee.
Inherits id, created_at, modified_at from base.
```

Inheritance: single inheritance via `extends`. Child
fields completely override parent fields with the
same name (no constraint merging). Circular chains
are detected and rejected.

Computed fields (declared in `computed:`) evaluate
at read time using the expression language from
spec §11. They are not persisted to the front matter;
queries can still reference them.

**Side by side.** A kind in mdsmith is the
user-facing concept that ties files to a rule
bundle and an optional schema; a type in mdbase is
the user-facing concept that ties files to a typed
schema. Both can carry a Markdown schema file.

| Aspect                    | mdsmith kinds                                                            | mdbase types                                 |
|---------------------------|--------------------------------------------------------------------------|----------------------------------------------|
| User-facing concept       | `kind` (per-file role)                                                   | `type` (per-file shape)                      |
| Kind/type definition      | `kinds:` map in `.mdsmith.yml`                                           | Markdown file in `_types/`                   |
| Schema file format        | Markdown `proto.md` (CUE in FM, headings in body)                        | Markdown in `_types/` (DSL in FM, body docs) |
| Schema travels with files | Yes — the `proto.md` is in the repo                                      | Yes — `_types/*.md` is in the vault          |
| Inheritance               | None across kinds; schemas compose via `<?include?>`                     | Single (`extends`)                           |
| Computed fields           | No                                                                       | Yes (expression-based, read-time)            |
| Bundles rule config       | Yes (deep-merge)                                                         | No (rules are mdsmith's, not mdbase's)       |
| Validates body structure  | Yes (heading template + `<?require?>`)                                   | No (body is documentation only)              |
| Multi-type per file       | Yes (kind list, deep-merged)                                             | Yes (multi-match, intersect constraints)     |
| Path constraint           | `kind-assignment` globs                                                  | `path_pattern` (validates and generates)     |
| Discoverability           | `mdsmith kinds` lists effective config; schema files are normal Markdown | List `_types/`                               |

A kind without a schema is still useful: it can
just toggle rule settings for a slice of files
(e.g. `proto` disables structural rules on
`proto.md` files themselves). A kind with a
schema points at a `proto.md` that validates
matched files. The two concepts compose via
deep-merge.

## 4. File-to-type / kind matching

**mdsmith.**
Two mechanisms (see CLAUDE.md):

- Front-matter `kinds: [name]` or `kind: name`
- `kind-assignment:` block in `.mdsmith.yml`:

  ```yaml
  kind-assignment:
    - glob: ["docs/api/**"]
      kinds: [api-doc]
  ```

A file can have any number of kinds; each is merged
in declaration order. There is no field-presence
auto-matching.

**mdbase.**
Three mechanisms (spec §6), evaluated in priority
order:

1. **Explicit declaration** — front-matter `type:` or
   `types:` (configurable via `explicit_type_keys`).
   Takes precedence; match rules are bypassed.
2. **`path_glob`** — glob in the type file's match
   block, e.g. `path_glob: "tasks/**/*.md"`.
3. **`fields_present`** — type matches when listed
   fields are non-null in the front matter.

Multi-condition rules combine with AND. Files can
match zero, one, or many types. When multiple types
match, fields are validated against the intersection
of their constraints (most restrictive wins; conflicts
become errors).

| Mechanism             | mdsmith       | mdbase                 |
|-----------------------|---------------|------------------------|
| Front-matter declare  | yes           | yes (highest priority) |
| Glob                  | yes           | yes                    |
| Field presence        | no            | yes                    |
| Field-value condition | no            | yes (`where:` block)   |
| Multi-match merge     | deep-merge    | constraint intersect   |
| Untyped allowed       | yes (no kind) | yes (no match)         |

mdbase wins on declarative power: a `where:` block
can match e.g. `status: open` files automatically.
mdsmith requires explicit kind tags or globs.

## 5. Field type system

### mdsmith: schema files via [MDS020][mds020]

Front-matter and body-structure validation both
happen through [MDS020 required-structure][mds020].
A schema is a Markdown file (typically named
`proto.md`) referenced from a kind in `.mdsmith.yml`:

```yaml
kinds:
  rule-readme:
    rules:
      required-structure:
        schema: internal/rules/proto.md
```

The schema file itself carries CUE expressions in
its YAML front matter (validating the document's
front matter) and a heading template in its body
(validating the document's heading structure).
Example schema:

```markdown
---
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
"settings?": {[string]: _}
---
<?require
filename: "[0-9]*-*/README.md"
?>

# ?

## Settings

## Examples

## Meta-Information
```

CUE in the front matter provides rich constraints
(regex, enum disjunctions, optional fields, value
coercion). The body provides the heading template
that documents must match. `<?require?>` declares
extra constraints (e.g. filename glob).
`<?include?>` lets schemas compose by splicing in
fragments. Errors surface as MDS020 diagnostics
with line/column anchors.

This means an mdsmith schema and an mdbase type
file have very similar shape: both are Markdown
files with declarative front matter, and both
travel with the project under version control.
The differences sit in scope and ownership:

- mdsmith schemas validate **both** front matter
  (CUE) **and** body heading structure (template).
- mdbase types validate only the front matter
  (typed fields, constraints, generated values),
  and the body is documentation for humans.
- mdsmith schemas are referenced from `kinds:` in
  `.mdsmith.yml`. They live anywhere; the
  project convention is one `proto.md` next to
  the files it describes.
- mdbase types live under a fixed `_types/`
  folder so they are discoverable from the file
  tree alone.

### mdbase: 12 field types

Spec §7 defines a fixed taxonomy:

| Type     | Constraints                                         |
|----------|-----------------------------------------------------|
| string   | `min_length`, `max_length`, regex `pattern`         |
| integer  | `min`, `max`                                        |
| number   | `min`, `max` (IEEE 754)                             |
| boolean  | accepts `yes`/`no` aliases, normalized on write     |
| date     | ISO 8601 `YYYY-MM-DD`, year range 0001–9999         |
| datetime | ISO 8601 with optional timezone, preserved on write |
| time     | `HH:MM` or `HH:MM:SS`                               |
| enum     | required `values:` list, case-sensitive             |
| list     | `items:` typing, `min_items`, `max_items`, `unique` |
| object   | `fields:` map, max nesting depth ≥ 16               |
| link     | `target:` type, `validate_exists`                   |
| any      | no validation; useful for migration                 |

Universal options on every field: `type`, `required`,
`default`, `generated`, `description`, `deprecated`,
`unique`. Null semantics are explicit: a default is
not applied when a field is present-but-null.

**Comparison.** Side-by-side breakdown:

| Concern                 | mdsmith (CUE)                                    | mdbase (DSL)                                    |
|-------------------------|--------------------------------------------------|-------------------------------------------------|
| Schema file format      | `proto.md` (CUE in FM, heading template in body) | `_types/<name>.md` (DSL in FM, docs in body)    |
| Validation language     | CUE in front matter                              | Inline YAML in type front matter                |
| Type discoverability    | Read the `proto.md` referenced by a kind         | Read `_types/*.md`                              |
| Built-in types          | Anything CUE supports                            | 12 fixed types                                  |
| Custom constraints      | Yes (full CUE)                                   | Limited to the per-type knobs                   |
| Inheritance             | CUE unification + schema `<?include?>`           | `extends:` single inheritance                   |
| Body structure check    | Yes (heading template, `<?require?>`)            | No (out of scope)                               |
| Computed fields         | No (CUE is purely structural)                    | Yes (expression-based)                          |
| Generated values        | No                                               | `generated:` (ULID, UUID, sequence, timestamps) |
| Deprecation flag        | No                                               | Yes (`deprecated: true`)                        |
| Cross-field constraints | Yes (CUE)                                        | Limited                                         |

CUE is the more powerful constraint language;
mdbase's DSL is simpler but covers the typical Markdown
collection well. mdbase has built-in support for
generated values (ULIDs and timestamps written on
create), which mdsmith lacks: mdsmith does not author
files, so it has no notion of write-time generation.

## 6. Validation model

**mdsmith.**
Per-rule severity (`error`, `warning`). Every rule
declares its own default severity. The CLI exits
non-zero only on errors; warnings are reported but
do not block.

CI gate via `mdsmith check .`:

```bash
mdsmith check .
# exit code 0 if no errors; non-zero on any error
```

Schema validation specifically lives in [MDS020][mds020].
A schema mismatch is an error. There is no global
"strict" toggle — strictness is encoded in each
schema (CUE either accepts or rejects).

**mdbase.**
Three global validation modes, set in `mdbase.yaml`
(spec §9):

| Mode    | Behavior                                     |
|---------|----------------------------------------------|
| `off`   | No validation; tools accept anything         |
| `warn`  | Reports issues; operations succeed (default) |
| `error` | Reports issues and aborts the operation      |

Plus a separate `default_strict:` (true / false /
"warn") that controls whether unknown front-matter
keys are flagged.

Validation covers six checks:

1. Required fields are present and non-null
2. Types match or coerce safely
3. Constraints (length, range, pattern, enum)
4. Strictness (unknown fields)
5. Link existence (when `validate_exists: true`)
6. ID uniqueness across the collection

**Side by side.** In summary:

| Concern               | mdsmith                    | mdbase                                  |
|-----------------------|----------------------------|-----------------------------------------|
| Severity granularity  | Per rule                   | Three global modes (off/warn/error)     |
| Strict unknown fields | Per CUE schema             | Global `default_strict`                 |
| Output format         | `text` or `json`; LSP wire | Code + path + field + line/column       |
| Error code count      | 54 rule IDs (one per rule) | 34 error codes (spec appendix C)        |
| Aborts on error       | exits non-zero             | aborts the CRUD op when mode = `error`  |
| Severity location     | declared by each rule      | global, with optional per-type override |

mdsmith's severity model is rule-local. mdbase's is
mode-global, except where `validation:` overrides
locally on a type or query. The mdsmith model is
more granular for linting; the mdbase model is
better aligned with CRUD operations that need a clear
"abort or proceed" decision.

## 7. Front-matter handling

| Aspect                        | mdsmith                                    | mdbase                                                  |
|-------------------------------|--------------------------------------------|---------------------------------------------------------|
| Format                        | YAML only (`---` fences)                   | YAML only (`---` fences)                                |
| Parser                        | `gopkg.in/yaml.v3` via `internal/yamlutil` | implementation-defined; spec mandates YAML 1.2 subset   |
| TOML / JSON FM                | not parsed                                 | not in spec                                             |
| Obsidian inline `key:: value` | not recognized                             | not in spec (but `compatibility` notes it)              |
| Anchor / alias attack         | rejected (security hardening)              | implementation-defined                                  |
| Effective FM (post-defaults)  | concept absent; rules see raw YAML         | yes — `effective frontmatter` includes defaults         |
| Computed fields               | n/a                                        | included in `effective frontmatter` for queries         |
| Null semantics                | YAML default                               | explicit: present-null bypasses defaults and uniqueness |

mdsmith treats front matter as plain YAML the rules
read directly. mdbase adds a layer: the **effective
frontmatter** is the raw YAML merged with defaults
and computed fields. Queries operate on the effective
view, not the raw bytes. This matters for tools that
want to see "the data as if all defaults were applied"
without rewriting the file on disk.

## 8. Query language

### mdsmith: CUE struct literal

```bash
mdsmith query 'status: "✅"' plan/
mdsmith query 'meta: {priority: int, status: "open"}' .
```

The argument is a CUE struct literal that the engine
unifies against each file's front matter
(`internal/query/query.go`). Unification succeeds
when the file's data is a subtype of the query
struct. `Match` adds an explicit existence check so
queries cannot match a file that simply omits a
field.

CUE's full expression language is available: regex,
disjunctions, optional fields, computed bounds. There
is no body search, no sort, no pagination — `query`
just emits matching paths.

### mdbase: Bases-compatible expression DSL

Spec §10 defines a richer model with clauses:

```yaml
types: [task]
folder: tasks
where: status == "open" && priority >= 3 && due <= today() + "7d"
order_by:
  - field: due
    direction: asc
limit: 20
offset: 0
```

Plus access namespaces:

- bare names → effective front matter (`status`)
- `file.*` → metadata (`file.mtime`, `file.size`,
  `file.path`)
- `file.body` → full-text search on raw Markdown
- `formula.*` → computed fields

Operators and methods (spec §11):

- comparison: `==`, `!=`, `<`, `>`, `<=`, `>=`
- logical: `&&`, `||`, `!`
- arithmetic: `+`, `-`, `*`, `/`, `%`
- null coalescing: `value ?? default`
- string methods: `.length`, `.contains`, `.startsWith`,
  `.endsWith`, `.lower`, `.upper`, `.trim`,
  `.matches(regex)`, `.replace(p, r)`
- list methods: `.length`, `.contains`, `.filter(expr)`,
  `.map(expr)`, `.reduce(expr, init)`
- date functions: `now()`, `today()`, `date(s)`,
  `datetime(s)`, with `.year`, `.month`, `.day`, etc.
- date arithmetic with duration strings: `"7d"`, `"1w"`,
  `"1M"` (months), `"1y"`, `"2h"`, `"30m"`, `"45s"`
  — case-sensitive (capital M = month, lowercase = minute)
- conditionals: `if(cond, then, else)`
- existence: `exists(field)`, `value.isEmpty()`,
  `value.isType("string"|"number"|...)`
- link traversal: `link.asFile().property` (max depth 10)
- type-mismatch errors return null and emit
  `type_error` rather than aborting the query

**Side by side.** In summary:

| Capability        | mdsmith               | mdbase                              |
|-------------------|-----------------------|-------------------------------------|
| Filter by FM      | yes (CUE unification) | yes (expression `where:`)           |
| Sort              | no                    | yes (`order_by`)                    |
| Pagination        | no                    | yes (`limit`/`offset`)              |
| Body search       | no                    | yes (`file.body`)                   |
| Cross-file fields | no                    | yes (`asFile()` traversal)          |
| Date arithmetic   | via CUE bounds        | first-class duration strings        |
| Aggregation       | no                    | yes (`Query+`: formulas, summaries) |
| Tooling           | one CLI flag          | CLI + library + LSP                 |

For ad-hoc CI filters ("which files have
`status: blocked`?"), mdsmith query is enough. For
running a vault as a database (showing all
overdue tasks grouped by assignee), mdbase wins.

### Concrete query examples

Eight worked queries against the same task corpus,
showing the syntactic and capability differences.
The corpus has FM like
`{ id, title, status, priority, due, assignee,
related_tasks }` per file.

**Q-A: simple equality filter.** List files where `status == "open"`.

```bash
# mdsmith
mdsmith query 'status: "open"' tasks/
```

```yaml
# mdbase
types: [task]
where: status == "open"
```

Both work today. The mdsmith form is a CUE
struct literal; mdbase uses an expression
string.

**Q-B: compound boolean.** Open tasks with priority at least 3.

```bash
# mdsmith — compound conditions in CUE need
# inequality bounds and conjunction inside one struct
mdsmith query 'status: "open", priority: int & >=3' tasks/
```

```yaml
# mdbase
types: [task]
where: status == "open" && priority >= 3
```

The mdbase syntax is more familiar; the CUE
form is exact and composes with existing
struct schemas.

**Q-C: date range — "due in the next 7 days".** Open tasks due in the next week.

```bash
# mdsmith — CUE has no first-class duration arithmetic
# in the query surface; users either compute the bound
# externally and substitute, or post-filter:
DUE_LIMIT="$(date -u -d '+7 days' +%Y-%m-%d)"
mdsmith query "status: \"open\", due: <= \"${DUE_LIMIT}\"" tasks/
```

```yaml
# mdbase
types: [task]
where: status == "open" && due <= today() + "7d"
```

mdbase wins on ergonomics here. The shell
substitution works for mdsmith but is awkward
in CI (every job recomputes the bound).
Closing this gap is Q-4 in
[learn-from-mdbase.md](learn-from-mdbase.md).

**Q-D: list contains — "tasks tagged urgent".**
Tasks whose `tags` list includes `urgent`.

```bash
# mdsmith — list membership via CUE list literal
mdsmith query 'tags: ["urgent", ...]' tasks/
```

```yaml
# mdbase
types: [task]
where: tags.contains("urgent")
```

Both work; the CUE form requires the list to
match exactly the literal; the variant
`'tags: [..., "urgent", ...]'` is needed for
"contains" rather than "starts with". mdbase's
method form is more readable.

**Q-E: cross-file traversal — "RFCs whose owner
is a senior engineer".** Filter on a field of
the linked person record.

```yaml
# mdbase: assumes person files have a level field
types: [rfc]
where: owner.asFile().level == "senior"
```

```text
# mdsmith: not expressible in `query` today.
# Workaround:
#   1. mdsmith query 'level: "senior"' people/
#   2. capture the file basenames as a set
#   3. mdsmith query "owner: <name>" rfcs/ for each
# This is L-5 / Q-6 in learn-from-mdbase.md.
```

**Q-F: body content — "tasks mentioning OIDC".**
Filter by a substring in the body.

```yaml
# mdbase
types: [task]
where: file.body.contains("OIDC")
```

```text
# mdsmith: not expressible in `query` today. The
# right answer is a native body-content matcher
# built into mdsmith, not a pipe to an external
# tool — mdsmith ships as a single static binary
# and should not require ripgrep or grep on PATH.
# Q-3 in learn-from-mdbase.md sketches a native
# `--body-contains REGEX` flag using Go's regexp
# engine and the existing parallel file walk.
```

**Q-G: sort and limit — "top 5 oldest open tasks".** The five oldest open tasks.

```yaml
# mdbase
types: [task]
where: status == "open"
order_by:
  - field: created
    direction: asc
limit: 5
```

```text
# mdsmith: not expressible in `query` today.
# Q-1 (`--order-by`) and Q-2 (`--limit`) in
# learn-from-mdbase.md add native flags;
# until they land, the user filters with
# `mdsmith query` and orders/limits in the
# calling script. The native answer is in
# mdsmith, not in a pipe.
```

Q-1 / Q-2 in
[learn-from-mdbase.md](learn-from-mdbase.md)
add `--order-by` and `--limit` flags.

**Q-H: aggregation — "count open tasks per
assignee".** Group by assignee, count.

```yaml
# mdbase
types: [task]
where: status == "open"
group_by: assignee
aggregate:
  - count
order_by: count desc
```

```text
# mdsmith: not expressible in `query` today.
# Q-5 in learn-from-mdbase.md sketches a native
# `--group-by` / `--aggregate` surface on
# `mdsmith query`. mdsmith should not require
# jq, awk, or other external tools to do
# aggregation — the binary ships standalone.
```

Aggregation lives in the mdbase DSL today;
mdsmith would grow a native equivalent if the
trigger fires.

**Reading across.** mdsmith's query covers the
common filter case ergonomically. mdbase
covers more inside the DSL — date arithmetic,
cross-file traversal, body search, aggregation
— at the cost of a richer language to learn.
The candidate plans (Q-1 through Q-5 in
[learn-from-mdbase.md](learn-from-mdbase.md))
all describe **native mdsmith** implementations
of these capabilities, not workarounds via
shell pipelines or external dependencies.
mdsmith's single-binary identity is preserved
either way: today by keeping `query` simple, or
later by adding the native operations alongside
the existing parallel-walk and FM-parse
infrastructure. For interactive vault
navigation where the user
is composing many queries quickly, mdbase pays
for its language complexity.

## 9. Link handling

**mdsmith.**
MDS027 (cross-file-reference-integrity) validates
Markdown links and same-file anchors:

- File links: `[text](path/to/file.md)` — checks the
  target file exists
- Heading anchors: `[text](file.md#section)` — checks
  the heading exists, using GitHub anchor slug rules
- Same-file anchors: `[text](#local)` — checks H1–H6
  in the current file
- External links (`http://`, `https://`, `mailto:`)
  are skipped

Settings: `include`, `exclude` globs;
`strict: true|false` (whether to also check
non-Markdown targets); `placeholders:` list (tokens
treated as opaque, e.g. `var-token`).

Wikilinks (`[[Page]]`) are not parsed — they pass
through as text. There is no link rewriting on file
rename.

**mdbase.**
Spec §8 defines link parsing, resolution, and
traversal as a first-class concern.

Three input formats:

- Wikilinks: `[[target]]`, `[[target|alias]]`,
  `[[target#anchor]]`
- Markdown links: `[text](path.md)`
- Bare paths: `./sibling.md`, `../parent/file.md`

Resolution (in order):

1. ID-field match (when type constraint exists)
2. Filename match in Markdown files
3. Tiebreakers: same-directory preference, shortest
   path, alphabetical order

Path sandboxing prevents escape attempts (so
`../../etc/passwd` cannot resolve outside the
collection). Non-Markdown files are not record
candidates for simple-name resolution.

Traversal: `link.asFile()` resolves the link and
returns the target file. Multi-hop is allowed up to
depth 10. Null propagation prevents errors during
incomplete chains.

**Side by side.** In summary:

| Capability                      | mdsmith              | mdbase                     |
|---------------------------------|----------------------|----------------------------|
| Markdown link existence check   | yes                  | yes (L4)                   |
| Same-file anchor check          | yes (#heading)       | yes                        |
| Cross-file heading anchor check | yes                  | yes                        |
| Wikilink parsing                | no (treated as text) | yes (`[[...]]`)            |
| Link aliases                    | n/a                  | yes (`[[t\|alias]]`)       |
| ID-field link resolution        | no                   | yes                        |
| Path sandboxing                 | yes                  | yes (L4)                   |
| Multi-hop traversal             | no                   | yes (`asFile()`, depth 10) |
| Auto-rewrite on rename          | no                   | yes (L5)                   |
| Backlink graph                  | no                   | yes (L5)                   |

mdbase is the link-aware tool. mdsmith catches broken
links but does not maintain the graph.

## 10. Cross-file integrity and rename

mdsmith MDS027 validates and reports
broken links. It does not rewrite anything: a rename
breaks links, MDS027 catches it on the next lint, the
human or LLM agent fixes them.

mdbase L5 (`References`) requires:

- **Rename with reference updates** — moving or
  renaming a file rewrites incoming wikilinks and
  Markdown links across the collection. Link styles
  are preserved (a wikilink stays a wikilink). ID-based
  links may skip rewriting if the target's `id_field`
  did not change.
- **Backlink computation** — a query like "what
  links to this file?" runs against the link graph.

Concurrency model: rename uses optimistic concurrency
via mtime checks. Reference updates are best-effort
file-by-file. A rename can succeed while individual
reference updates fail (reported per-file).

| Capability              | mdsmith | mdbase               |
|-------------------------|---------|----------------------|
| Detect broken link      | yes     | yes                  |
| Auto-rewrite incoming   | no      | yes (L5)             |
| Link-style preservation | n/a     | yes                  |
| Backlinks query         | no      | yes (L5, via cache)  |
| Concurrency safety      | n/a     | optimistic via mtime |

Practical impact: in mdbase you can rename a file in
one command and downstream links stay green. In
mdsmith you must do the rewrite manually (or via a
separate tool) and re-lint.

## 11. CRUD operations

mdsmith does not author files. It edits content
inside files via `mdsmith fix`, but it does not
create or delete `.md` files. It does not have a
notion of file-creation operations at all.

mdbase L1 defines a full CRUD surface (spec §12):

| Operation | Behavior                                                                                                                 |
|-----------|--------------------------------------------------------------------------------------------------------------------------|
| Create    | New file with type inference, defaults, and generated values; validates before write; atomic                             |
| Read      | Loads file, parses front matter and body, resolves types, returns metadata + effective FM                                |
| Update    | Merges new field values; recalculates types; applies generated strategies (e.g. `now_on_write`); validates merged result |
| Delete    | Removes file; optionally warns about incoming references that would break                                                |
| Rename    | Moves file; optionally rewrites references (L5)                                                                          |
| Batch     | `--where` filter to target many files; validate-all then apply, or best-effort                                           |

`--dry-run` mode validates without writing. The
`generated:` field strategies cover ULID, UUID,
sequences, and timestamps — useful for files created
programmatically.

| Capability             | mdsmith                         | mdbase                         |
|------------------------|---------------------------------|--------------------------------|
| Create file            | no                              | yes (L1)                       |
| Read with effective FM | indirect (via lint)             | yes (L1)                       |
| Update field           | no (only fix-driven body edits) | yes (L1)                       |
| Delete file            | no                              | yes (L1)                       |
| Rename                 | no                              | yes (L5 with refs, L1 without) |
| Batch ops              | per-rule; no `--where`          | yes (`--where` selector)       |

This is the cleanest "different layer" cut. mdsmith
is read-and-fix; mdbase is read-write-CRUD.

## 12. Generated and regenerable content

### mdsmith: directives

Block-form processing instructions. Each pairs with
a rule that detects drift and a fix that regenerates
the body. Four active body-generating directives,
all using the form
`<?name [params]?>...body...<?/name?>`:

| Directive     | Rule             | Generates                                                 |
|---------------|------------------|-----------------------------------------------------------|
| `<?catalog?>` | [MDS019][mds019] | Table of files matching a glob, with FM fields as columns |
| `<?include?>` | [MDS021][mds021] | Inline content from another Markdown file                 |
| `<?toc?>`     | [MDS038][mds038] | Nested heading list with GitHub anchors                   |
| `<?build?>`   | [MDS039][mds039] | Body from a recipe template (no execution today)          |

Heading-template enforcement is **not** a
directive; it is the [MDS020][mds020] rule
applied to a kind whose
`required-structure.schema:` points at a schema
file (typically `proto.md`). The schema's body
carries the required heading skeleton; the rule
checks that real files match. Schemas can
themselves use `<?include?>` to compose
fragments, and a schema-only PI `<?require?>`
declares filename / structural constraints
(spec'd in
`internal/rules/MDS020-required-structure/README.md`).

Plus support directives:

- `<?allow-empty-section?>` — opt out of the
  empty-section rule per-section
- `<?ignore?>` — opt out of all rules for a span

The fix engine (`internal/fix/fix.go`) regenerates
each directive body. When `mdsmith fix` runs, it
re-evaluates the directive parameters and rewrites
the body between markers. The source file remains
valid Markdown at all times — readers see the
generated content even when the linter has not run.

### mdbase: not in scope

Spec §0–15 do not define a generation directive
syntax. mdbase covers the data layer only. A
collection that needs generated TOCs or catalogs
relies on a static-site generator or a separate tool
(such as mdsmith).

**Side by side.** In summary:

| Capability         | mdsmith                    | mdbase |
|--------------------|----------------------------|--------|
| Directive syntax   | `<?name?>...<?/name?>`     | none   |
| Catalog (FM table) | yes                        | no     |
| Include            | yes                        | no     |
| TOC                | yes                        | no     |
| Build artifacts    | yes (recipes; no exec yet) | no     |
| Drift detection    | yes (per directive rule)   | no     |
| Auto-regenerate    | yes (`mdsmith fix`)        | no     |

This is an mdsmith-only layer. A team using mdbase
who also wants generated tables of contents would
need to run a second tool, or write their own tooling
that uses the mdbase library.

## 13. Prose, readability, and structure rules

mdsmith ships 54 rules organized loosely by category.
mdbase has none — it does not lint prose at all.

### Prose & readability (mdsmith only)

| Rule   | Concern                                                     |
|--------|-------------------------------------------------------------|
| MDS023 | Paragraph readability (ARI grade index)                     |
| MDS024 | Paragraph structure (max sentences, max words per sentence) |
| MDS028 | Token budget (LLM-context-window aware)                     |
| MDS029 | Conciseness scoring (experimental, classifier-based)        |
| MDS037 | Duplicated content across files                             |

### Structure (mdsmith only)

| Rule       | Concern                                                |
|------------|--------------------------------------------------------|
| MDS001     | Line length (default 80, exclude code/tables/urls)     |
| MDS002–005 | Heading style, increment, first line, no duplicates    |
| MDS006–009 | Whitespace (trailing spaces, hard tabs, blank lines)   |
| MDS010–011 | Fenced code style and language tag                     |
| MDS012     | No bare URLs                                           |
| MDS013–015 | Blank lines around headings, lists, fenced code        |
| MDS016     | List indent                                            |
| MDS017–018 | Heading punctuation, no emphasis as heading            |
| MDS022     | Max file length (lines)                                |
| MDS025–026 | Table format and table readability                     |
| MDS030     | Empty section body                                     |
| MDS031     | Unclosed code block                                    |
| MDS032     | No empty alt text                                      |
| MDS033     | Directory structure                                    |
| MDS034     | Markdown flavor (CommonMark / GFM gate)                |
| MDS035     | TOC token (`[TOC]` and friends)                        |
| MDS036     | Max section length                                     |
| MDS041–054 | Inline HTML, emphasis, lists, ambiguous emphasis, etc. |

### Cross-file & generation (mdsmith only)

| Rule   | Concern                                             |
|--------|-----------------------------------------------------|
| MDS019 | Catalog directive                                   |
| MDS020 | Required structure (CUE schema validation)          |
| MDS021 | Include directive                                   |
| MDS027 | Cross-file reference integrity                      |
| MDS038 | TOC directive                                       |
| MDS039 | Build directive                                     |
| MDS040 | Recipe safety (build command syntax check)          |
| MDS048 | Git-hook sync (`.gitattributes` and pre-merge hook) |

**Observation.** mdbase has no equivalent. A vault using mdbase for
data layering still needs a linter for prose quality,
heading conventions, and table formatting. mdsmith
fits this gap.

## 14. Conventions and presets

mdsmith ships three built-in conventions
(`internal/rules/markdownflavor/conventions.go`):

| Convention | Markdown flavor | Notable presets                                            |
|------------|-----------------|------------------------------------------------------------|
| portable   | CommonMark      | no inline HTML; emphasis bold=`*` italic=`_`; HR=`---`     |
| github     | GFM             | inline HTML allow [details, summary]; emphasis as portable |
| plain      | CommonMark      | minimal: only the strictest baseline                       |

Selecting a convention sets all the relevant
formatting rules at once; user `rules:` still wins
on top via deep-merge.

Plan 113 covers user-defined conventions: teams
package their own preset bundles. Today only the
three built-ins exist.

mdbase has no equivalent. The spec does not describe
preset bundles; each implementation chooses what
to ship by default. Validation modes (off/warn/error)
are not bundles, just toggles.

## 15. Cache, watch, daemon

| Concern          | mdsmith                  | mdbase                                         |
|------------------|--------------------------|------------------------------------------------|
| Persistent cache | none                     | optional SQLite at `.mdbase/index.sqlite` (L6) |
| Cache rebuild    | n/a                      | `mdbase cache rebuild`                         |
| Staleness check  | n/a                      | mtime + content hash                           |
| Watch mode       | none                     | event stream (L6); 7 event types               |
| Debounce         | n/a                      | 100–500ms window                               |
| Daemon           | none                     | implementation-defined (LSP is the closest)    |
| LSP cache        | per-session config cache | per-session config cache                       |

The cache is the biggest runtime difference. mdbase
expects to be useful at vault sizes of 10k+ files;
the SQLite index makes that practical. mdsmith
re-reads everything every run; on small repos this
is a feature (deterministic, no stale state). On
huge corpuses it is a limitation.

mdbase's seven watch events: `file_created`,
`file_modified`, `file_deleted`, `file_renamed`,
`type_definition_updated`, `config_changed`,
`validation_error`. Listeners can react in real time
to any of these.

## 16. CLI surface

**mdsmith.** mdsmith side:

```text
mdsmith check [files...]            # lint
mdsmith fix [files...]              # autofix in place
mdsmith query <expr> [files...]     # CUE-based filter
mdsmith init                        # write default .mdsmith.yml
mdsmith metrics rank [files...]     # rank by metric
mdsmith metrics list                # available metrics
mdsmith kinds                       # show effective kinds per file
mdsmith help [topic]                # offline rule docs
mdsmith lsp                         # JSON-RPC over stdio
mdsmith merge-driver install [globs] # git merge driver
mdsmith merge-driver run …          # invoked by git
mdsmith pre-merge-commit install    # git hook installer
mdsmith version                     # build version
```

### mdbase (CLI implementation)

The spec does not mandate a CLI shape, but the
TypeScript CLI ships approximately:

```text
mdbase init                # bootstrap mdbase.yaml + _types/
mdbase types               # list / inspect types
mdbase validate [path]     # run schema validation
mdbase query <query>       # run a Bases-syntax query
mdbase create <type>       # create a new typed file
mdbase update <path>       # update a file's fields
mdbase rename <old> <new>  # rename + update refs (L5)
mdbase delete <path>       # delete a file
mdbase cache rebuild       # rebuild SQLite index (L6)
mdbase cache clear         # drop cache
mdbase watch               # event stream (L6)
mdbase migrate             # apply migration manifests (L6)
mdbase infer-types         # bootstrap types from existing FM
```

(Exact subcommands depend on the implementation;
spec §0 lists "CLI tools for querying and manipulating
collections" as a target audience but does not fix
command names.)

**Side by side.** In summary:

| Capability           | mdsmith                    | mdbase (TS CLI)              |
|----------------------|----------------------------|------------------------------|
| Lint                 | `check`                    | (out of scope, per spec)     |
| Autofix              | `fix`                      | n/a                          |
| Schema validate      | `check` (MDS020)           | `validate`                   |
| Query                | `query`                    | `query`                      |
| Create file          | n/a                        | `create`                     |
| Rename + update refs | n/a                        | `rename`                     |
| Watch                | n/a                        | `watch`                      |
| Cache rebuild        | n/a                        | `cache rebuild`              |
| LSP                  | `lsp`                      | separate `mdbase-lsp` binary |
| Git merge driver     | `merge-driver install`     | n/a                          |
| Git pre-merge hook   | `pre-merge-commit install` | n/a                          |
| Initialize project   | `init`                     | `init`                       |
| Built-in rule docs   | `help <rule-id>`           | (spec docs only)             |

## 17. LSP / editor integration

**mdsmith today.**
`mdsmith lsp` runs JSON-RPC 2.0 over stdio. Used by
the VS Code extension and any LSP-aware editor or
agent. Implements:

- `textDocument/publishDiagnostics` — diagnostic
  push on save / change
- `textDocument/codeAction` — `quickFix` per
  diagnostic, `sourceFixAll` for whole-file fix
- `workspace/configuration` — reads `mdsmith.config`
  (config path) and `mdsmith.run` (`onSave`,
  `onType`, `off`)
- `workspace/didChangeWatchedFiles` — invalidates
  cached config when `.mdsmith.yml` changes

Not implemented today: hover, completions,
signature help, definition, references, rename. The
shipped server is diagnostic-and-fix only.

Source: `internal/lsp/server.go`,
`internal/lsp/diagnostics.go`,
`internal/lsp/protocol.go`.

**mdsmith planned (plans 122 and 131).**
Two open plans extend the LSP toward parity with
typical code-LSPs and toward explicit support for
agent-driven navigation (Claude Code's LSP tool,
Neovim, Helix):

- **Plan 122** ([`plan/122_vscode-hover-and-palette.md`][plan122])
  adds `textDocument/hover` for rule IDs and
  directive names, surfacing the offline rule docs
  and directive parameter help inline.
- **Plan 131** is tracked in [PR #238][pr238]; the
  plan file lives on that branch and is not yet on
  `main`. It adds symbol navigation:
  `documentSymbol`, `definition`, `implementation`,
  `references`, `workspace/symbol`, plus call
  hierarchy (`prepareCallHierarchy`,
  `incomingCalls`, `outgoingCalls`). The design
  models a Markdown file as a "function" and
  outbound references (links, includes, catalog
  matches, build targets) as "calls", so an agent
  can ask "who depends on this runbook?" or "what
  does this overview embed?" through standard LSP
  methods.

Plan 131 explicitly maps the nine LSP methods that
Claude Code's LSP tool exposes (symbol, definition,
implementation, hover, references, workspace
symbol, call hierarchy in / out / prepare) onto
the existing AST and link graph. This is the
"LSP for agents" arc.

Performance budgets in plan 131: cold index build
under 1s on 1,000 files, incremental update under
20ms per `didChange`. Memory cap reuses plan 121's
512 MB `GOMEMLIMIT`.

**mdbase.**
A separate `mdbase-lsp` binary (Rust) provides
language-server features over the typed collection.
Per the project README it covers:

- diagnostics (validation errors)
- completions (field names, enum values, type names)
- hover info (type definitions, field constraints)
- go-to-definition (link target, type)

The Rust LSP is independent from the TypeScript
reference implementation. It reads `mdbase.yaml`
and the collection directly. Symbol-level
navigation (documentSymbol, references, workspace
symbol, call hierarchy) is not advertised in the
project README at the time of writing.

**Side by side.** Three columns: mdsmith today,
mdsmith after plans 122 + 131 land, and
mdbase-lsp today.

| LSP feature              | mdsmith today | mdsmith planned | mdbase-lsp       |
|--------------------------|---------------|-----------------|------------------|
| `publishDiagnostics`     | yes           | yes             | yes              |
| `codeAction` (quick fix) | yes           | yes             | n/a (no autofix) |
| `codeAction` (fix all)   | yes           | yes             | n/a              |
| `hover`                  | no            | yes (plan 122)  | yes              |
| `completion`             | no            | not yet planned | yes              |
| `documentSymbol`         | no            | yes (plan 131)  | partial          |
| `definition`             | no            | yes (plan 131)  | yes              |
| `implementation`         | no            | yes (plan 131)  | unknown          |
| `references`             | no            | yes (plan 131)  | unknown          |
| `workspace/symbol`       | no            | yes (plan 131)  | unknown          |
| `prepareCallHierarchy`   | no            | yes (plan 131)  | no               |
| `incomingCalls`          | no            | yes (plan 131)  | no               |
| `outgoingCalls`          | no            | yes (plan 131)  | no               |
| `rename`                 | no            | candidate (L-3) | yes (L5)         |
| `signatureHelp`          | no            | no              | no               |
| `semanticTokens`         | no            | no              | no               |
| `inlayHint`              | no            | no              | no               |
| `codeLens`               | no            | no              | no               |
| `didChangeWatchedFiles`  | yes (`.yml`)  | yes (`**/*.md`) | yes              |

The two LSPs are non-overlapping in practice
today. A team running both gets diagnostics from
each, code actions from mdsmith, and completion +
navigation from mdbase. Once plan 131 lands,
mdsmith covers the navigation surface as well —
specifically the call-hierarchy view on the
include/catalog/build graph that mdbase does not
model.

**LSP for AI agents.** Both projects are
explicitly positioned for use by AI agents over
LSP, but with different framings:

- mdbase-lsp gives an agent the typed-vault view:
  "what fields does this type allow", "where is
  this link's target". The schema is the agent's
  reading frame.
- mdsmith plan 131 gives an agent the
  document-graph view: outline, anchors, includes,
  catalog matches, build targets. The link graph
  is the agent's reading frame.

For an agent rewriting prose with structural
awareness, mdsmith's planned outline + call
hierarchy is the closer fit. For an agent
reasoning about typed records (queries,
constraints, generated values), mdbase-lsp wins.
A team using both serves both reading frames over
LSP without leaving the editor protocol.

## 18. Output formats

mdsmith (`internal/output/`):

- `text` — human-readable with file:line:col, rule
  ID, message, source snippet, ANSI color (toggle
  with `--no-color`)
- `json` — array of diagnostic objects with file,
  line, column, severity, rule-id, rule-name, message
- LSP wire format via the LSP server
- `--explain` flag attaches per-leaf rule provenance

No SARIF support today.

mdbase: spec §9 mandates each diagnostic carry
`path`, `field`, `code`, `message`, `severity`, with
optional `line`, `column` for LSP-style reporting.
Output format is implementation-defined; reference
impls produce JSON.

| Format         | mdsmith  | mdbase reference impl |
|----------------|----------|-----------------------|
| Text (TTY)     | yes      | yes                   |
| JSON           | yes      | yes                   |
| LSP wire       | yes      | yes (`mdbase-lsp`)    |
| SARIF          | no       | no (not specified)    |
| GitHub Actions | via JSON | via JSON              |

## 19. Conformance and extension model

**mdsmith.**
No conformance tiers — there is one binary.
Extension is via plan-driven internal rule additions
(`internal/rules/<id>-<name>/`). User-defined
conventions (plan 113) and user-defined rules are
not yet shipped. A team that wants a new rule today
contributes upstream.

**mdbase.**
Six conformance levels (spec §14):

| Level | Name       | Adds                                                   |
|-------|------------|--------------------------------------------------------|
| 1     | Core       | Config, frontmatter, types, validation, CRUD           |
| 2     | Matching   | Path globs, field presence, multi-type                 |
| 3     | Querying   | Filter, sort, limit, expressions, body search          |
| 4     | Links      | Wikilinks, markdown, bare paths, resolution, traversal |
| 5     | References | Rename with reference updates, backlinks               |
| 6     | Full       | SQLite cache + staleness, batch ops, watch, migrations |

Each level carries a YAML test suite. An impl declares
its level, e.g.:

```text
mdbase-tool v1.0.0
Conformance: Level 4 (Links)
Specification: 0.2.1
```

A new conforming impl can ship with only Level 1
support and grow upward; users know what to expect.

**Comparison.** Side-by-side breakdown:

| Aspect                 | mdsmith            | mdbase                              |
|------------------------|--------------------|-------------------------------------|
| Spec / impl separation | no (single tool)   | yes (spec-first)                    |
| Extension model        | upstream PRs       | new conforming impl                 |
| Versioning             | semver of binary   | spec version + impl semver          |
| Multi-impl interop     | n/a                | yes (any L_n impl reads same files) |
| User-defined rules     | not yet (plan 113) | not in spec; per-impl               |

mdbase's spec-first model is more robust to
ecosystem fragmentation. mdsmith's single-impl model
is simpler to reason about but creates a single
point of fork should the project stall.

## 20. Security posture

**mdsmith.** mdsmith side:

- Single static binary; no runtime dependencies
- Offline at every phase
- YAML billion-laughs guard: anchors and aliases are
  rejected (`internal/yamlutil/yamlutil.go`)
- Path traversal guards in build directive (rejects
  `..` in output paths)
- Recipe safety (MDS040) validates command syntax at
  lint time; build directive does not execute today
- Process-level GOMEMLIMIT bounds CUE evaluation
- Atomic writes in fix mode
- 10-finding adversarial review documented in
  `docs/security/2026-04-05-adversarial-markdown.md`

**mdbase.**
The spec is silent on most security concerns. What
it does mandate:

- Path sandboxing in link resolution (spec §8) —
  links cannot resolve outside the collection root
- Optimistic concurrency via mtime (spec §12) —
  prevents lost updates
- `path_traversal` error code (Appendix C) — implies
  impls reject escape attempts
- File-watch debouncing (spec §15) — prevents event
  storms, indirectly bounds resource use

No spec-level requirements on YAML billion-laughs,
adversarial input, or memory bounds. Each impl
chooses its own posture. The TypeScript impl in
particular brings a Node module graph as supply-chain
surface.

| Hardening            | mdsmith           | mdbase spec    | mdbase TS impl |
|----------------------|-------------------|----------------|----------------|
| Single binary        | yes               | n/a            | no (npm tree)  |
| Offline              | yes               | mandated       | yes            |
| YAML alias rejection | yes               | unspecified    | impl-defined   |
| Path traversal block | yes               | mandated (§8)  | yes            |
| ANSI escape sanitize | yes               | unspecified    | unknown        |
| Memory bounds        | yes (GOMEMLIMIT)  | unspecified    | impl-defined   |
| Adversarial review   | yes (10 findings) | n/a            | unknown        |
| Atomic writes        | yes               | mandated (§12) | yes            |

For untrusted Markdown (PRs from external
contributors, third-party content) mdsmith ships
with the harder posture today. An mdbase impl can
match it but would need to declare its hardening
explicitly.

## 21. Determinism and idempotence

| Property                         | mdsmith           | mdbase                         |
|----------------------------------|-------------------|--------------------------------|
| Same input → same output         | yes               | yes                            |
| Order-independent                | yes (stable sort) | yes (deterministic resolution) |
| Repeated `fix` converges         | yes (multi-pass)  | n/a (no fix layer)             |
| Cache invalidation deterministic | n/a (no cache)    | mtime + hash                   |
| Multi-impl agree                 | n/a               | yes (test suite)               |

Both tools are deterministic by design. mdbase's
multi-impl model carries more test obligation: every
conforming impl must produce the same answer for
the conformance test cases.

## 22. Comparison summary

| Domain                  | Owner        |
|-------------------------|--------------|
| Prose readability       | mdsmith only |
| Heading and list rules  | mdsmith only |
| Whitespace fixing       | mdsmith only |
| Code-fence enforcement  | mdsmith only |
| Token budget for LLMs   | mdsmith only |
| Generated TOC / catalog | mdsmith only |
| Include directive       | mdsmith only |
| Build directive         | mdsmith only |
| Git merge driver        | mdsmith only |
| Front-matter typing     | shared       |
| Front-matter querying   | shared       |
| Schema validation       | shared       |
| File-to-role assignment | shared       |
| Wikilink parsing        | mdbase only  |
| Rename refactoring      | mdbase only  |
| Backlink graph          | mdbase only  |
| SQLite index            | mdbase only  |
| Watch / event stream    | mdbase only  |
| CRUD operations         | mdbase only  |
| Migrations              | mdbase only  |
| Computed fields         | mdbase only  |
| Generated values        | mdbase only  |
| LSP completions / hover | mdbase only  |
| LSP diagnostics         | shared       |
| LSP code actions        | mdsmith only |

[mds019]: ../../../internal/rules/MDS019-catalog/README.md
[mds020]: ../../../internal/rules/MDS020-required-structure/README.md
[mds021]: ../../../internal/rules/MDS021-include/README.md
[mds038]: ../../../internal/rules/MDS038-toc/README.md
[mds039]: ../../../internal/rules/MDS039-build/README.md
[pr238]: https://github.com/jeduden/mdsmith/pull/238
[plan122]: ../../../plan/122_vscode-hover-and-palette.md
