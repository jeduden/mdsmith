---
summary: >-
  Systematic enumeration of mdbase capabilities mdsmith does not yet
  have, with a per-gap mini-plan covering goal, design sketch, surface,
  effort, trigger, and open questions.
---
# What mdsmith can learn from mdbase

This document is a research exercise, not a roadmap.
mdsmith does not have a closed feature set; it is
evolving. Every gap below is a candidate direction.
The point of the exercise is to map the candidates,
sketch each one concretely enough to argue about,
and name the **trigger** under which shipping it
becomes worth the cost — not to declare any of them
in or out.

## Method

1. Walk every section of [features.md](features.md).
2. For each row where mdbase has a capability and
   mdsmith does not, note it as a gap with a short
   identifier (e.g. `S-1`, `Q-3`).
3. For each gap, record:

  - an **effort** estimate (S / M / L: a day, a
     week, a month)
  - a **trigger**: a concrete condition under
     which shipping the gap is worth the cost
     (real user demand, a profiling result, a
     dependency from another shipped feature, a
     coherence pull from a subsystem)
  - whether work is already in flight (linking
     the open plan or PR)

4. Write a mini-plan per gap with:

  - **Goal** — one sentence
  - **Trigger** — when shipping becomes worth it
  - **Sketch** — proposed mechanism, including
     concrete syntax where it helps
  - **Surface** — what new config / CLI / output
     the user sees
  - **Effort** — S / M / L
  - **Depends on** — gaps or existing plans this
     builds on
  - **Open questions** — things a real plan would
     have to settle

## Capability tables

Tables split by area for readability. Each row
names a candidate direction with effort and
trigger; the mini-plan below carries the design
sketch.

### Schema language (S)

| ID  | Gap                                                        | Effort | Trigger / status                                                                                 |
|-----|------------------------------------------------------------|--------|--------------------------------------------------------------------------------------------------|
| S-1 | Inline schema in `kinds:`                                  | M      | one team asks for a schema without a separate `proto.md`                                         |
| S-2 | Named field-type taxonomy as CUE shortcuts                 | S      | S-1 lands and the YAML shorthand becomes useful                                                  |
| S-3 | Schema inheritance (`extends:`)                            | S–M    | a project has three or more schemas with shared fields                                           |
| S-4 | Computed fields surfaced in catalog / query                | M      | a real catalog wants a field derived from FM                                                     |
| S-5 | Generated values on write (ULID, timestamps)               | M      | mdsmith grows a `create` or scaffolding subcommand                                               |
| S-6 | Field deprecation flag                                     | S      | a real schema migration needs to deprecate without breaking                                      |
| S-7 | Rich body schema (sections, fields, refs, acronyms, index) | L      | a project authors structured Markdown (deck, runbook, training) where current MDS020 falls short |

### File-to-kind matching (M)

| ID  | Gap                                      | Effort | Trigger / status                                                 |
|-----|------------------------------------------|--------|------------------------------------------------------------------|
| M-1 | Field-presence kind assignment           | S      | a project tags >100 files with `kinds:` boilerplate              |
| M-2 | Field-value kind assignment (`where:`)   | S–M    | a project wants derived kinds (e.g. open-task vs archived-task)  |
| M-3 | `path-pattern` (validates and generates) | S      | a project enforces filename conventions today via custom scripts |

### CRUD and rename (C)

| ID  | Gap                            | Effort | Trigger / status                                                           |
|-----|--------------------------------|--------|----------------------------------------------------------------------------|
| C-1 | Create typed file              | M      | a user-facing demand for scaffolding from inside mdsmith (e.g. agent loop) |
| C-2 | Update fields from CLI         | M      | C-1 lands; consistency pulls update along                                  |
| C-3 | Delete with broken-ref warning | S      | C-1 / C-2 ship; deletion completes the CRUD surface                        |
| C-4 | Atomic rename + ref rewrite    | L      | rename pain shows up in real workflows (issue threads, agent retries)      |
| C-5 | Batch ops with `--where`       | M      | a CRUD surface (C-1..C-4) needs scoping beyond globs                       |
| C-6 | Dry-run mode for fix           | S      | first agent or CI user asks to preview changes                             |

### Links (L)

| ID  | Gap                                   | Effort | Trigger / status                                                         |
|-----|---------------------------------------|--------|--------------------------------------------------------------------------|
| L-1 | Wikilink awareness                    | M      | the first Obsidian-vault user files a "wikilinks aren't validated" issue |
| L-2 | ID-field link resolution              | M      | a project imposes its own ID system and wants link-by-ID                 |
| L-3 | Auto-rewrite incoming links on rename | L      | (covered by C-4)                                                         |
| L-4 | Backlinks subcommand                  | S      | first agent or doc-team request for "what links to X"                    |
| L-5 | Multi-hop link traversal in queries   | M      | a project wants `[[A]] → asFile().status` semantics in `query`           |

### Query (Q)

| ID  | Gap                                   | Effort | Trigger / status                                                |
|-----|---------------------------------------|--------|-----------------------------------------------------------------|
| Q-1 | Sort (`--order-by`)                   | S      | first script piping `query` through `sort` complains            |
| Q-2 | Pagination (`--limit` / `--offset`)   | S      | first script wraps `query` with `head` / `tail`                 |
| Q-3 | Body full-text search in query        | S–M    | first user request for FM + body filter combined                |
| Q-4 | Date arithmetic with duration strings | M      | due-date / mtime queries become common in CI                    |
| Q-5 | Aggregations (formulas, summaries)    | L      | a recurring "group plans by status" pattern shows up in user CI |
| Q-6 | Cross-file traversal in queries       | M      | L-1 + L-4 ship; traversal becomes the obvious next step         |
| Q-7 | Bases-compatible expression syntax    | M      | enough Obsidian users ask for it that two query languages pays  |

### Cache, watch, migrations, conformance, LSP

| ID  | Gap                                    | Effort | Trigger / status                                                                                                       |
|-----|----------------------------------------|--------|------------------------------------------------------------------------------------------------------------------------|
| P-1 | Persistent on-disk index               | L      | profiling shows parse cost dominates after netting out OS cache and any in-memory index (see aggregation-use-cases.md) |
| P-2 | Watch mode beyond LSP per-session      | M      | a CLI workflow needs cross-process freshness (rare today)                                                              |
| V-1 | Migration manifests                    | L      | mdsmith grows write-back to FM (S-5 / C-2) and breaking schema changes hurt                                            |
| X-1 | Spec-first / multi-impl model          | XL     | a second implementation appears (fork, or upstream split); revisit then                                                |
| H-1 | LSP hover for rules / directives       | —      | in-flight (plan 122)                                                                                                   |
| H-2 | LSP symbol navigation + call hierarchy | —      | in-flight (plan 131, PR #238)                                                                                          |

## Schema language plans

### S-1: Inline schema in the `kinds` map

**Goal.** Let `kinds:` carry a schema directly,
without forcing every kind to point at a separate
`proto.md`.

**Trigger.** A team or contributor asks for an inline schema rather than
maintaining a separate `proto.md` per kind, especially when the
schema is small enough to read at a glance from the config.

**Sketch.** Extend the kind config to accept an
optional `schema:` block alongside (or instead of)
`required-structure.schema:`:

```yaml
kinds:
  task:
    rules:
      line-length:
        max: 100
    schema:
      frontmatter:
        id: '=~"^TASK-[0-9]{4}$"'
        status: '"open" | "in-progress" | "done"'
        priority: 'int & >=1 & <=5'
        "due?": "string"  # ISO date
      structure:
        - "# {title}"
        - "## Goal"
        - "## Acceptance"
      require:
        filename: "TASK-[0-9]+.md"
```

The schema engine receives a parsed AST whether
it came from inline YAML or from a referenced
`proto.md`. The current `proto.md` mechanism stays
as the path for schemas that include heading
templates with rich body content; inline schemas
cover the common case.

**Surface.** Three things change for the user:

- New `schema:` key in each kind
- `mdsmith kinds` command shows whether a kind's
  schema is inline or file-referenced
- Existing `required-structure.schema:` still works

**Effort.** M (1–2 weeks). Touches config types,
the schema loader, and `mdsmith kinds` output.
Mostly mechanical.

**Depends on.** Nothing.

**Open questions.** Should inline `structure:`
support the full directive set (`<?include?>` for
fragment composition)? Probably yes, for parity.

### S-2: Named field-type taxonomy as CUE shortcuts

**Goal.** Make CUE schemas more ergonomic by
shipping reusable definitions for the common types
mdbase names directly (date, datetime, enum, link).

**Trigger.** S-1 lands and the YAML shorthand for common types (`date`,
`email`, `url`) starts paying more than the cost of teaching
users two ways to spell the same constraint.

**Sketch.** Ship a small CUE library that
schemas can import. The library lives at a
publicly importable module path (e.g.
`github.com/jeduden/mdsmith/types`); the source
in this repo would sit somewhere like
`cue/types/types.cue` or be exposed via
mdsmith's CUE overlay (Go's `internal/` path
is not importable from outside, so a public
module path is required for schemas in user
projects). The library defines common patterns:

```cue
package mdsmith

#date:     =~"^\\d{4}-\\d{2}-\\d{2}$"
#datetime: =~"^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}"
#email:    =~"^[^@]+@[^@]+$"
#url:      =~"^https?://"
#filename: =~"^[A-Za-z0-9_-]+\\.md$"
```

Schemas import the package and reference the
definitions:

```cue
package proto

import "github.com/jeduden/mdsmith/types"

created:  types.#date
modified: types.#datetime
```

(The exact module path is illustrative; this
mini-plan would need to settle the canonical
location and how mdsmith ships the overlay so
external schemas can resolve the import.)

Optionally expose a more compact YAML shorthand in
inline schemas (S-1):

```yaml
schema:
  frontmatter:
    created: date     # resolves to types.#date
    homepage: url
    contact: email
```

**Surface.** A new package import path; optional
shorthand in inline schemas.

**Effort.** S (a few days). Defining the patterns
is small; wiring the shorthand is the bulk.

**Depends on.** S-1 makes the YAML shorthand
useful; without it the patterns are still helpful
in `proto.md` schemas.

**Open questions.** What's the canonical list?
Start with the nine mdbase scalar types (string,
int, number, bool, date, datetime, time, enum,
link) and grow from real use.

### S-3: Schema inheritance via `extends`

**Goal.** Let one kind / schema extend another so
common front-matter fields live in a base schema.

**Trigger.** A project carries three or more schemas with overlapping
required fields (e.g. all RFCs share `id`, `status`, `created`,
`authors`) and the duplication has begun to drift.

**Sketch.** Two parts:

- **In `kinds:`** — `extends:` lists parent kinds
  whose rule overrides apply before this one (today
  this is implicit deep-merge by declaration order;
  `extends:` makes it explicit).
- **In schemas** — CUE already supports
  unification, so `import` + struct embedding
  covers it. For inline schemas (S-1), extend the
  block:

  ```yaml
  schema:
    extends: [base]
    frontmatter:
      status: '"open" | "done"'
  ```

  The schema engine merges the parent's CUE struct
  with the child's, with child-wins on overlapping
  keys (mdbase semantics).

**Surface.** New `extends:` key in kind / inline
schema; documented in the file-kinds guide.

**Effort.** S–M. CUE already handles unification;
the lift is exposing the merge in the inline DSL.

**Depends on.** S-1.

**Open questions.** Single inheritance (mdbase) or
multiple? Multiple is more powerful but harder to
debug. Recommend single for parity, with `<?include?>`
covering composition.

### S-4: Computed fields in catalog / query

**Goal.** Let users define expressions that derive
a field from other front-matter fields, surface
the result in `<?catalog?>` columns and `mdsmith
query`, and never persist it back to disk.

**Trigger.** A real catalog or query case wants a field derived from FM
rather than stored — for example `days_since_review` — strongly
enough that users are working around the gap.

**Sketch.** Computed fields live in the schema
(inline or proto.md) under a `computed:` block:

```yaml
schema:
  frontmatter:
    due: string       # ISO date
  computed:
    days_overdue:
      type: int
      expr: time.Parse("2006-01-02", due).Before(time.Now())
```

The expression language reuses CUE; an extra small
helper for date arithmetic (Q-4) makes typical
cases readable. Catalog references computed values
as `{computed.days_overdue}`. Query treats them as
read-only fields.

**Surface.** New `computed:` block in schemas;
catalog template extension.

**Effort.** M. CUE evaluation per file is
manageable; the catalog templating change is small.

**Depends on.** S-1 (for inline syntax), Q-4 (for
date arithmetic to be useful).

**Open questions.** Performance — naive evaluation
runs the expression per file per render. Cache
across a single `mdsmith fix` run. Beyond that,
out of scope.

### S-6: Field deprecation flag

**Goal.** Let a schema mark a field deprecated;
mdsmith warns when it appears in front matter.

**Trigger.** A schema change needs to deprecate a field without breaking
existing files, and the absence of a soft-deprecation signal
forces a hard removal.

**Sketch.** In the schema, `deprecated: true` on a
field. MDS020 emits a Warning (not an Error) when
the field is present.

```yaml
schema:
  frontmatter:
    legacy_owner:
      type: string
      deprecated: true
      message: "use 'owner' instead"
```

**Surface.** Schema field-level `deprecated:` and
`message:` keys; new MDS020 sub-diagnostic.

**Effort.** S.

**Depends on.** S-1.

**Open questions.** None worth blocking on.

### S-7: Rich body schema for structural document types

**Goal.** Type the body of structured Markdown
documents beyond the heading sequence: nested
sections with per-field content rules (word and
character counts, forbidden / required text
patterns, scoped skip rules), cross-reference
validation, acronym tracking, and document-wide
index generation. Closes the
"structural body typing" gap noted in
[query-and-types.md](query-and-types.md) §
"What's missing in both".

**Trigger.** Two or more projects in the same
repo (or in adjacent repos a team maintains)
author structured Markdown beyond docs and RFCs
— runbooks, training material, presentation
decks, regulatory documents, change-log
manifests — and the constraint vocabulary
starts to repeat across their custom lint
scripts. That repeat is the signal: a generic
schema language earns its keep when teams
discover they want the same eight rules in
three different places.

**Sketch.** A schema describes a document's
body the way MDS020 today describes a heading
template, but reaches deeper. The form is
genuinely open — the language could be richer
YAML in `proto.md` front matter, extended CUE,
or a small DSL designed for this. The shape
below is an illustrative skeleton, not a
finished design; it sketches what the schema
language would need to express. The example is
a runbook because runbooks are common,
generic, and exercise every capability without
a vocabulary tied to one domain.

```yaml
# illustrative — not a final syntax

sections:
  - heading: "## Overview"
    required: true

  - heading: "## Symptoms"
    required: true
    aliases: ["## Indicators"]

  - heading: "## Diagnosis"
    required: true
    children:
      pattern: "### Step {n}"
      sequential: true               # no gaps; no duplicates
      fields:
        - heading: "#### Check"
          required: true
          max_words: 50
        - heading: "#### Expected"
          required: true
        - heading: "#### If different"
          required: false
          required_patterns:
            - pattern: "see Step \\d+"
              error: "missing forward reference"
              skip_indices: [-1]     # last step exempt

  - heading: "## Pass criteria"
    required: true
    fields:
      - heading: "#### Indicator"
        required: true
        max_words: 30
        forbidden_starts: ["We ", "The system "]
        forbidden_contains: ["should", "may", "might"]

  - heading: "## References"
    required: false

# Cross-reference validation: "Step 4" mentions
# must resolve to an actual "### Step 4" heading.
cross_references:
  - pattern: "\\bStep (\\d+)\\b"
    must_match: "### Step {n}"
    skip_lines_matching: "^> "       # skip blockquoted text

# Flag acronyms before their first expansion;
# allowlist covers infrastructure terms.
acronym_tracking:
  known_safe: [API, HTTP, TLS, DNS, IAM, CDN, SLA, JSON]
  scope: ["#### Check", "#### Expected"]

# Generate a JSON side-output for downstream tooling.
index:
  output: ".runbook-index.json"
  include: [step_map, cross_ref_graph, word_counts]
```

The shape exercises five capabilities current
MDS020 does not address:

1. **Hierarchical body structure.** Nested
   sections with required / optional /
   repeatable semantics; child patterns
   (`### Step {n}`) with sequential or
   non-sequential numbering.
2. **Per-field content rules.** Word counts,
   character counts, forbidden starts and
   contents, required text patterns, scoped
   skip rules (e.g. "the last step is exempt
   from the forward-reference requirement").
3. **Cross-reference validation.** Text like
   "Step 4" must resolve to a real
   `### Step 4` heading; broken refs are
   diagnostics. Skip rules let blockquoted
   stale text pass through.
4. **Acronym tracking.** First-use detection
   with a known-safe allowlist and a
   per-section scope.
5. **Index generation.** A JSON side-output
   of the parse pass — step map, cross-ref
   graph, per-field word counts. Downstream
   tooling consumes the index instead of
   re-parsing the document.

**Surface.** A new schema format alongside (or
extending) the current `proto.md` body. Either
an extension of MDS020 (richer schema language
inside the existing rule) or a sister rule that
runs alongside. `mdsmith fix` regenerates the
index file when the document changes;
`mdsmith check` reports violations with file /
line / column anchors.

**Effort.** L. The largest schema-side
candidate. The schema language needs careful
design; the validator does real work (per-field
rules, cross-reference resolution, acronym
tracking are each a small subsystem); index
generation adds an output surface.

**Depends on.** S-1 (inline schemas) as a
stepping stone — the YAML form here is in
spirit an inline schema. Possibly S-3
(`extends`) so common section patterns can be
shared across kinds.

**Open questions.** Six things a real plan would
have to settle:

- **Schema language choice.** YAML in `proto.md`
  front matter (close to mdbase's style) vs.
  extending CUE (close to current mdsmith) vs.
  a new DSL designed for this. Each has costs.
- **Composition with MDS020.** Is this a richer
  MDS020 schema, or a sister rule that runs
  alongside? A project might want both
  heading-template constraints (MDS020 today)
  and the richer body rules on the same files.
- **Index output location and format.** JSON
  next to the source file, gitignored cache,
  or somewhere else? mdbase's
  `effective_json` per-file cache is one
  shape; a per-document `.runbook-index.json`
  is another.
- **Cross-reference scope.** Refs must resolve,
  but blockquoted text and version-history
  notes break this. The schema example has
  `skip_lines_matching`; that's a configurable
  escape hatch. Does it generalize?
- **Relationship to mdbase types.** mdbase
  types declare FM shape; this proposal
  declares body structure. They are
  orthogonal in scope, but a project running
  both wants one definition file, not two.
  How would the schema bridge
  ([interop.md §7](interop.md)) cover the
  body half?
- **Performance.** Per-field word counts and
  cross-reference resolution scale with file
  size. Runbooks, decks, training modules
  fit; books would not.

**Why this matters in the comparison.** mdbase's
data-layer typing and mdsmith's heading-template
schemas both stop at the boundary of "rich body
structure". A user with a 50-slide deck or a
long structured runbook gets the same answer
from either tool today: write your own lint
script. S-7 would close this gap from mdsmith's
side; mdbase doesn't have a path to closing it
without going outside its data-layer scope.

This is the most ambitious schema candidate in
this catalogue. It is not trivial — the
sketch above is itself a "needs work" design.
But it is the right shape for the gap, and the
direction is consistent with mdsmith's existing
broader-rule-set approach: declarative schema
language, lint-time enforcement, drift detection,
auto-fix where the change is regenerable
(the index is regenerable; word-count
violations are not).

## File-to-kind matching plans

### M-1: Field-presence kind assignment

**Goal.** Auto-assign a kind when specific
front-matter fields are present, removing the need
for an explicit `kind:` tag.

**Trigger.** A project tags more than ~100 files with `kinds:` boilerplate
and the duplication is painful enough that auto-assignment by
FM shape is worth a new mechanism.

**Sketch.** Extend `kind-assignment:` with a
`fields-present:` selector:

```yaml
kind-assignment:
  - glob: ["docs/**"]
    kinds: [doc]
  - fields-present: [status, priority, assignee]
    kinds: [task]
```

Selectors combine with AND; multiple entries
combine with OR (matching the existing glob
semantics). A file matching no entry stays
untyped, like today.

**Surface.** New `fields-present:` key in
`kind-assignment:` entries; documented in the
file-kinds guide.

**Effort.** S.

**Depends on.** Nothing.

**Open questions.** Should presence-based matching
fire on null values (present but null)? Mirror
mdbase's "non-null required" semantics.

### M-2: Field-value kind assignment via `where`

**Goal.** Auto-assign a kind based on a CUE
expression over front matter, e.g. "files where
status is 'open'".

**Trigger.** A project wants derived kinds — `open-task` vs `archived-task`,
draft vs published — and globs aren't expressive enough to
capture the distinction.

**Sketch.** Extend `kind-assignment:` with a
`where:` selector that takes a CUE struct
literal — same syntax as `mdsmith query`:

```yaml
kind-assignment:
  - where: 'kind: "plan", status: "in-progress"'
    kinds: [active-plan]
```

Useful for derived kinds: "active plans get a
stricter token budget than archived plans". A
config-time CUE compile per kind keeps the cost
low.

**Surface.** New `where:` key in
`kind-assignment:` entries.

**Effort.** S–M.

**Depends on.** Nothing.

**Open questions.** Performance at scale; if
`where:` runs per file per kind, n×m cost. A
trie-based prefilter on the keys mentioned in
`where:` mitigates. Defer until the feature is in
real use.

### M-3: `path-pattern` (validates and generates)

**Goal.** Let a kind declare a filename pattern
that validates existing files (does the path
match?) and templates new ones (where should a
new file live?).

**Trigger.** A project enforces filename conventions today via custom CI
scripts and would benefit from declaring the pattern alongside
the kind.

**Sketch.** Add `path-pattern:` to a kind:

```yaml
kinds:
  plan:
    rules:
      required-structure:
        schema: plan/proto.md
    path-pattern: "plan/{id:int}_{slug}.md"
```

Validation: a file claiming `kind: plan` whose
path doesn't fit `plan/<int>_<slug>.md` produces
an MDS020 Error. Templating: `mdsmith query` could
emit candidate paths from this pattern (low
priority).

**Surface.** New `path-pattern:` key in kind
config; new MDS020 sub-diagnostic.

**Effort.** S.

**Depends on.** Nothing.

**Open questions.** Field substitution syntax —
braces with type tags as above, or simpler globs?
Match what mdbase does for portability.

## Link plans

### L-1: Wikilink awareness

**Goal.** Make MDS027 understand `[[Page]]`,
`[[Page|alias]]`, `[[Page#anchor]]` so wikilink-
heavy vaults can use mdsmith without flagging
every wikilink as text.

**Trigger.** The first Obsidian-vault user files a "wikilinks aren't
validated" issue, or a docs team adopts wikilinks for navigation
and finds MDS027 silent.

**Sketch.** Add a wikilink parser to the lint
pipeline (alongside the existing Markdown link
extractor in `internal/lint/`). Resolution rules
follow goldmark's link-resolution conventions plus
mdbase spec §8 (filename match in `.md` files;
sandboxed to repo root). Configurable via
`cross-file-reference-integrity.wikilinks:`:

```yaml
rules:
  cross-file-reference-integrity:
    wikilinks: true        # default false today
    wikilink-style: simple # | obsidian
```

Two styles because Obsidian and other tools
disagree on tiebreakers and aliases.

**Surface.** New rule settings; new diagnostic
flavors for broken wikilinks.

**Effort.** M. Parsing is small; resolution rules
need test coverage across Obsidian / Foam /
Logseq variants.

**Depends on.** Nothing.

**Open questions.** Should mdsmith canonicalize
wikilinks (rewrite to Markdown links on `fix`)?
Probably no — that's a refactor, not a lint, and
breaks Obsidian for users who want wikilinks.

### L-3 / C-4: Atomic rename + reference rewrite

**Goal.** A `mdsmith refactor rename <old> <new>`
subcommand that moves a file and rewrites every
incoming link.

**Trigger.** Rename pain shows up in real workflows — recurring PRs that
touch tens of files for one rename, or agent retries looping on
broken links — strongly enough that a one-shot refactor command
pays.

**Sketch.** New subcommand:

```bash
mdsmith refactor rename docs/old.md docs/new.md
mdsmith refactor rename --heading "Old name" --to "New name" docs/file.md
```

Two modes:

- **File rename.** Moves `docs/old.md` to
  `docs/new.md`. Rewrites every `[text](docs/old.md)`,
  `[text](docs/old.md#anchor)`, and `[[old]]` in
  the workspace. Anchors normalized via the same
  GitHub-slug rules MDS027 uses.
- **Heading rename.** Updates the heading text in
  the source file and rewrites every cross-file
  anchor link pointing at the old slug.

`--dry-run` prints the proposed edits without
writing. Optimistic-concurrency check uses mtime
(file changed on disk during rename → abort).

**Surface.** New `mdsmith refactor` subcommand
group with `rename` and (later) other refactors.

**Effort.** L. Rename is the new write surface
mdsmith hasn't had before. Test matrix is large
(wikilinks, anchors, edge cases like links in
fenced code blocks).

**Depends on.** L-1 (wikilinks) for full coverage.

**Open questions.** Should rename also update
`<?include?>` and `<?build?>` directive paths?
Yes, for consistency. Should it touch front-matter
fields containing paths (`source: file.md`)?
Probably not without explicit opt-in — they aren't
syntactically links.

### L-4: Backlinks subcommand

**Goal.** A `mdsmith backlinks <file>` command
that lists every workspace file with a link to
the target.

**Trigger.** First agent or docs-team request for "what links to this file?"
The link graph becomes load-bearing for navigation or impact
analysis.

**Sketch.** Reuses the link graph that MDS027
already builds during a check pass:

```bash
mdsmith backlinks docs/api.md
# docs/index.md:14: [API reference](api.md)
# docs/getting-started.md:42: [api docs](./api.md)
# plan/045_api-overhaul.md:8: [api](../docs/api.md)

mdsmith backlinks docs/api.md#authentication
# docs/index.md:18
# docs/security.md:33
```

JSON output for agents (`--format json`).

**Surface.** New `mdsmith backlinks` subcommand;
JSON output spec.

**Effort.** S. Most of the engine is in MDS027
already; surface it as a CLI command.

**Depends on.** L-1 to cover wikilinks (otherwise
the result misses Obsidian-style backlinks).

**Open questions.** Performance at 10k files —
the link graph builds fast but the output may be
unwieldy. `--limit N` plus filtering by glob
covers the common case.

## Query plans

### Q-1, Q-2: Sort and pagination

**Goal.** Support `--order-by FIELD` and
`--limit N` / `--offset N` on `mdsmith query`.

**Trigger.** First script that pipes `mdsmith query` through `sort` / `head` /
`tail` files an issue asking for native flags. Low bar; small fix.

**Sketch.** New flags:

```bash
mdsmith query 'status: "open"' --order-by priority --order desc --limit 20
mdsmith query 'kind: "task"' --order-by due --offset 50 --limit 50
```

Sorts after CUE unification (i.e. on filtered
result set). Stable sort with deterministic
secondary key (path) so output is reproducible.

**Surface.** Three new flags on `query`; JSON
output unchanged.

**Effort.** S (a day or two).

**Depends on.** Nothing.

**Open questions.** Sort by computed fields
(S-4)? Yes, when S-4 lands.

### Q-3: Body full-text search in query

**Goal.** Filter files by body content as well as
front matter.

**Trigger.** First user asks for a query that combines FM filtering with body
content — typically agents or CI scripts looking for `TODO`
markers in scoped subsets of the corpus.

**Sketch.** New flag `--body-contains REGEX`:

```bash
mdsmith query 'kind: "doc"' --body-contains 'TODO|FIXME'
```

Anchors to body only (not front matter, not
inside fenced code unless `--include-code`). The
regex engine is Go's `regexp` package.

**Surface.** New flag; JSON output includes a
`matches` array per file when `--match-context`
set.

**Effort.** S–M.

**Depends on.** Nothing.

**Open questions.** Should it support multiple
patterns (AND / OR)? One pattern keeps the
surface small; users can pipe through `grep` for
more complex needs.

### Q-4: Date arithmetic with duration strings

**Goal.** Make CUE queries readable for "due in
the next 7 days":

```bash
mdsmith query 'due: <=time.Add(time.Now(), "7d24h")' tasks/
```

**Trigger.** Due-date, mtime, or created-since queries become common in CI,
and CUE's lack of duration arithmetic forces verbose timestamp
construction.

**Sketch.** Add a small CUE package
`internal/cue/time.cue` that defines `Add`,
`Sub`, and parsing helpers, exposed in the query
namespace by default. Match the duration-string
grammar mdbase uses (`"7d"`, `"1w"`, `"1M"`,
`"1y"`, `"2h"`, `"30m"`, `"45s"`; capital M for
months, lowercase for minutes — error on
ambiguous unit).

The CUE package wraps `time.Parse` /
`time.ParseDuration` with month / year support
that Go's `time` lacks; clamp month-overflow to
the last day of the target month, matching
mdbase.

**Surface.** New CUE helpers visible in `query`
expressions; documented in
`docs/reference/cli/query.md`.

**Effort.** M.

**Depends on.** Nothing.

**Open questions.** Time zone — query against
local time or UTC? mdbase uses an `mdbase.yaml`-
configurable IANA zone. mdsmith could read a
top-level `timezone:` config key.

### Q-7: Bases-compatible expression syntax

**Goal.** Optionally accept Obsidian Bases-style
expressions in `mdsmith query`, so users coming
from a vault don't have to learn CUE for simple
filters.

**Trigger.** Enough Obsidian users ask for Bases-style filters to overcome
the cost of supporting two query languages. Likely never, but
the sketch is here so a real request lands on solid ground.

**Sketch.** Add a `--lang bases` flag:

```bash
mdsmith query --lang bases 'status == "open" && priority >= 3'
mdsmith query 'status: "open"'   # CUE, current default
```

Translation happens at the boundary: parse Bases
syntax into a CUE struct literal and unify as
today. Only the subset that maps cleanly: `==`,
`!=`, `<`, `>`, `<=`, `>=`, `&&`, `||`, `!`,
plus `.startsWith` / `.contains` / `.matches` on
strings.

**Surface.** New `--lang` flag; new doc page on
the supported subset.

**Effort.** M. Parser is small; the work is the
test matrix.

**Depends on.** Nothing.

**Open questions.** Worth the cost? Argument for:
lower onboarding for Obsidian users. Argument
against: two query languages, two failure modes,
two doc surfaces. **Recommendation: defer until a
real user asks for it.** The exploration here is
primarily evaluative. CUE is more powerful and
already documented; Bases is more familiar to
some. If we do add it, structure it as a
translation pass, not a parallel implementation.

## Operational plan

### C-6: `--dry-run` for fix

**Goal.** Show what `mdsmith fix` would change
without writing.

**Trigger.** First agent or CI user asks to preview changes before writing —
typically when agents run `mdsmith fix` autonomously and want a
safety net.

**Sketch.** New flag `--dry-run`:

```bash
mdsmith fix --dry-run docs/
# docs/api.md: would fix 3 violations (MDS001, MDS006, MDS019)
# docs/index.md: would regenerate <?catalog?> body
```

Emits the same diagnostics shape as `check` plus
a per-file summary of fixed-vs-unfixed counts.
Combine with `--format json` for tooling.

**Surface.** New `--dry-run` flag.

**Effort.** S. The fix engine already produces a
fixed-content buffer before writing; gate the
write.

**Depends on.** Nothing.

**Open questions.** None.

## Mini-plans-lite for the heavier directions

Several directions cross the line from
"read-and-fix" into write, persistence, or
multi-impl territory. They are fully on the table;
they just carry larger trade-offs and clearer
triggers. Each gets a brief sketch and the
condition under which it would pay.

### S-5: Generated values on write

**Goal.** Support `generated:` strategies (ULID,
UUID, sequence, `now_on_write`) that fill a field
when a typed file is created.

**Trigger.** mdsmith grows a `create` or
scaffolding subcommand (C-1) and users want
ULID-keyed records or `created` timestamps filled
automatically. Without C-1 there is no write
moment for these to fire.

**Sketch.** Schema field block adds
`generated: ulid | uuid | sequence | now_on_write`.
The create command applies the strategy at write
time; the lint engine treats generated fields as
present-in-effective-FM whether or not they are
on disk.

**Effort.** M after C-1.
**Depends on.** C-1.

### C-1: Create typed file (`mdsmith create`)

**Goal.** Scaffold a new typed file from a kind's
schema, filling defaults and generated values.

**Trigger.** A user-facing demand for scaffolding
from inside mdsmith — an agent loop that wants to
issue one command, a docs team that wants a
templated `new-rfc` flow without external tooling.

**Sketch.** New `mdsmith create <kind> [path]`
subcommand. Reads the kind's schema, prompts for
required fields without defaults (or accepts
`--field key=value`), writes the file atomically.
The body uses the schema's heading template if
present.

**Effort.** M.
**Depends on.** S-1 makes the schema legible
inline; without it the command still works, just
reads from `proto.md`.

### C-2: Update fields from CLI

**Goal.** Edit one or more FM fields in a file
without opening an editor.

**Trigger.** C-1 lands and consistency pulls
update along. Or an agent loop wants to flip
`status: open` → `status: done` programmatically.

**Sketch.** New `mdsmith update <path> --field key=value`.
Validates against the file's effective kind /
schema before writing. Atomic write with mtime
optimistic-concurrency check.

**Effort.** M.
**Depends on.** C-1 (shared write surface).

### C-3: Delete with broken-ref warning

**Goal.** Delete a file and warn about incoming
references that will break.

**Trigger.** C-1 / C-2 ship; deletion completes
the CRUD surface. Without the other two it is a
small odd one-off.

**Sketch.** `mdsmith delete <path>` runs MDS027's
link graph in reverse, lists incoming references,
asks for confirmation (or `--force`), then
removes the file.

**Effort.** S after the link-graph work for L-4.
**Depends on.** L-4.

### C-5: Batch ops with `--where`

**Goal.** Apply create / update / delete across
files matching a query.

**Trigger.** A CRUD surface (C-1..C-4) has
shipped and users want to scope batch edits
beyond what globs express.

**Sketch.** `mdsmith update --where 'status: "open"' --field reviewer=alice`.
Validates all matched files first, then applies;
fails fast on validation errors unless `--continue`.
`--dry-run` previews.

**Effort.** M after C-2.
**Depends on.** C-2, C-6.

### L-2: ID-field link resolution

**Goal.** Resolve `[[chen-2024]]` to a file
whose `id_field` equals `chen-2024`, regardless
of filename.

**Trigger.** A project imposes its own ID system
(citation keys, ticket IDs) and wants link-by-ID
that survives renames.

**Sketch.** Config knob
`cross-file-reference-integrity.id-field: id`.
When the link target has no path separator and
no `.md` extension, MDS027 first tries ID-field
match, then filename match. Same wikilink-style
resolution rules as mdbase L4.

**Effort.** M.
**Depends on.** L-1 (the wikilink form is the
primary user surface).

### L-5: Multi-hop link traversal in queries

**Goal.** Let `mdsmith query` walk a link to its
target and read the target's FM, e.g.
`assignee.asFile().role`.

**Trigger.** L-1 + L-4 ship and users start asking
"filter tasks by assignee.role" — i.e. queries
that cross the link graph rather than just FM.

**Sketch.** Add `.asFile()` to the CUE query
namespace. Resolves the link in front matter,
returns the target's effective FM as a struct.
Depth limit (default 5). Null propagation on
broken links.

**Effort.** M.
**Depends on.** L-1, L-4. Possibly P-1 for
performance at scale (each hop reads another
file).

### Q-5: Aggregations (formulas, summaries)

**Goal.** Group, count, sum, min/max, percentile
across a query result set.

**Trigger.** A recurring "group plans by status,
count each" pattern shows up in user CI scripts —
typically when teams use `mdsmith query` as a
lightweight reporting layer.

**Sketch.** A new flag combo, e.g.

```bash
mdsmith query 'kind: "plan"' \
  --group-by status \
  --aggregate count,priority:avg
```

Output is a tabular result, JSON-serializable.
Reuses the in-memory result set produced by Q-1
through Q-4.

**Effort.** L.
**Depends on.** Q-1 (sort), Q-3 (body filter).
This is the strongest argument for an index: at
scale, computing aggregations from a fresh parse
pass per query is the costly pattern. See
[the aggregation use cases doc](aggregation-use-cases.md)
for a deeper exploration of where the index pays
and where stateless approaches still hold up.

### Q-6: Cross-file traversal in queries

**Goal.** Query expressions that follow link
fields into other files (the query-language form
of L-5).

**Trigger.** L-5 ships and users ask for the
same in batch queries.

**Sketch.** Same `.asFile()` accessor in CUE
expressions, evaluated per-file with the depth
limit.

**Effort.** included in L-5.
**Depends on.** L-5.

### P-1: Persistent on-disk index

**Goal.** A cache of parsed FM, link edges, and
computed fields that survives across CLI runs.

**Trigger.** A persistent cache earns its cost
only after surviving three filters: OS-cache
(repeated runs are already RAM-fast through the
page cache), sync-check (validating the on-disk
cache against the filesystem isn't free —
50,000 files takes ~500ms of `stat` alone), and
in-memory-with-priority (a long-lived process
gets most of the win without the operational
baggage). The deeper walk-through is in
[aggregation-use-cases.md §"What an index
actually costs"](aggregation-use-cases.md). The
short version: ship this when the *parsed*
index is what's expensive, not the raw read,
and a long-lived process can't already cover
the access pattern.

**Sketch.** See the next section.

**Effort.** L.
**Depends on.** Whichever feature triggers it.

### P-2: Watch mode beyond LSP per-session

**Goal.** A persistent watcher that reflects file
changes into a shared cache or event stream
between CLI invocations.

**Trigger.** A CLI workflow needs cross-process
freshness — for example, a long-running agent
that fires `mdsmith query` repeatedly and the
mdsmith side wants warm state. Today the LSP
session covers editor cases.

**Sketch.** Optional `mdsmith watch` daemon that
runs alongside the project, exposing a Unix
socket for queries. Other CLI invocations check
for the socket, use it if present, fall back to
stateless run otherwise.

**Effort.** M.
**Depends on.** P-1 (a daemon without a cache is
just a re-runner).

### V-1: Migration manifests

**Goal.** Declare schema-version deltas and
backfill expressions, applied via `mdsmith migrate`.

**Trigger.** mdsmith grows write-back to FM
(via S-5 / C-2), and breaking schema changes
start hurting real projects with hundreds of
already-typed files.

**Sketch.** YAML manifests under a
`<schemas>/_migrations/` folder. Each names
`from_version`, `to_version`, and a list of
field-level transforms (rename, default backfill,
type coerce). `mdsmith migrate` walks files of
the affected kinds and applies transforms with
optimistic-concurrency mtime checks.

**Effort.** L.
**Depends on.** C-2.

### X-1: Spec-first / multi-impl model

**Goal.** Promote mdsmith's behavior to a
specification so other implementations can
conform.

**Trigger.** A second implementation appears —
either a fork that needs to merge back, or an
upstream split (e.g. a Rust port). Until then
the cost of spec maintenance has no payoff.

**Sketch.** Extract the rule semantics, schema
DSL, query language, and directive grammar into
a versioned spec under `spec/`. Add a
conformance test suite in YAML. Tier
implementations by capability ("conformance
level") the way mdbase does.

**Effort.** XL.
**Depends on.** Real second-impl pressure.

### A closer look at P-1 (SQLite cache)

The one-line rationale above hides a real design
question worth pulling apart, since the cache is
mdbase's biggest runtime difference from mdsmith.

**Why SQLite at all.** mdbase carries an index of
files, parsed front matter, computed-field
results, and link edges. At vault scale (5k–50k
files) re-reading every file on every query is
slow: seconds per query versus milliseconds. The
spec calls SQLite "Recommended" because it gives
ACID writes, queryable indexes, single-file
storage, and ubiquity, with the trade-off that
the cache file is binary and not human-readable
(spec §13). Some impls might choose a JSON line
file or an in-memory map; SQLite is the default
because the queries are relational.

**Staleness check at startup.** A correct cache
must invalidate when source files change.
mdbase's spec §13 mandates two signals:

- **Modification time (mtime).** Cheap. Used as
  the first-pass check. If a file's mtime is
  newer than the cached entry's, re-parse it.
- **Content hash.** Used when mtime is
  unreliable (network filesystems, build systems
  that touch files without changing content). On
  a hash mismatch the entry is rebuilt.

The cache is rebuildable from the files alone, so
"correct on stale" is acceptable: the worst case
is an extra parse pass at startup. The cache is
also deletable without data loss — `.mdbase/` can
be removed at any time.

A read-only command like `mdbase query` walks
every entry and verifies mtime against the cached
record before trusting it. If the cache is large
relative to the workspace this becomes the
bottleneck; impls add a watcher (spec §15) so the
cache stays current between queries.

**What queries actually run on the cache.** Three
shapes dominate in mdbase's CRUD and query
surfaces:

1. **Front-matter filtering.** "All files where
   `status == 'open'` and `priority >= 3`." The
   cache stores parsed FM as columns or a JSON
   blob with a generated indexing layer; SQLite's
   `WHERE` does the rest. Equivalent to
   `SELECT path FROM files WHERE status='open' AND priority>=3`.
2. **Backlink lookup.** "What files link to
   `docs/api.md` (or its `auth` heading)?" Stored
   as a normalized link-edge table:
   `(source_path, target_path, target_anchor, link_kind)`.
   A point query finds incoming edges in one
   index lookup.
3. **ID resolution.** "Resolve `[[chen-2024]]` to
   a file." A unique index on the configured
   `id_field` makes this O(log n) instead of a
   full scan.

Plus the L4 traversal (`link.asFile().property`)
which is recursive and benefits from the
denormalized FM in the cache.

**Alternatives mdsmith could consider.** If
mdsmith ever needed an index — for backlinks
(L-4), wikilinks (L-1), or fast `query` over
large repos — these are the candidate
implementations, ranked by complexity:

1. **No cache, parallel re-read.** What mdsmith
   does today. Reads files in parallel, parses
   each, builds an in-memory link graph for the
   duration of the run. Determinism is high,
   memory is bounded, but cold-start time scales
   linearly with file count. Fine to ~5k files.
2. **In-memory index with mtime invalidation,
   process-scoped.** The LSP server already does
   this for compiled config. Extend to a link
   graph and FM map. Survives one editor session;
   does not survive process exit. Good fit for
   LSP, no fit for one-shot CLI.
3. **Memory-mapped file index** (e.g.,
   FlatBuffers or a Go `gob` blob). Persistent
   across runs, bounded format, no SQL. Requires
   a custom query API; no `WHERE`-clause power.
   Fast load (mmap is constant-time) but every
   query rescans the chunk it cares about. Good
   for backlinks (point lookup) and tag indexes;
   bad for FM filtering with multiple predicates.
4. **Embedded key-value store** (BoltDB,
   Badger). Persistent, transactional, no
   external dependency. Schema is hand-rolled
   like option 3. Easier to evolve than mmap.
   No SQL means complex predicates need
   application-side filtering after a key scan.
5. **SQLite (mdbase's choice).** Persistent,
   ACID, full SQL, mature ecosystem.
   `modernc.org/sqlite` is a pure-Go driver so a
   static binary stays static. Trade-off: schema
   migrations and an extra dependency surface.
6. **Full-text search engine** (Bleve,
   Tantivy via cgo). Useful only if Q-3 (body
   FTS) becomes central. Overkill otherwise.

**Where mdsmith might land.** The choice depends
on which trigger fires. Option 2 (in-memory,
process-scoped) is already implicit in the LSP
server and is the cheapest extension for plan 131
needs (`documentSymbol`, `references`, call
hierarchy). For one-shot CLI use, option 1
(parallel re-read with no cache) holds up to
roughly 5k files on commodity hardware; beyond
that the choice between options 3, 4, and 5
depends on which queries dominate.

The strongest argument for SQLite (option 5) is
relational aggregation (Q-5): grouping, counting,
joining across types. If mdsmith grows in that
direction the SQL surface pays for itself. If
mdsmith stays predominantly in
filter-and-emit-paths territory, option 4
(BoltDB / Badger) gives most of the persistence
without the SQL maintenance cost.

A pragmatic sequencing:

- **Today** — option 1 (no cache) covers the
  CLI; option 2 (in-memory) covers the LSP.
- **First trigger** — when a real profiling
  case shows cold-start as the bottleneck, or
  when L-4 / Q-5 demand cross-file traversal at
  scale, prototype option 4 and measure.
- **Second trigger** — if the prototype's
  hand-rolled queries become unwieldy, escalate
  to option 5 (SQLite). The migration is well-
  understood: BoltDB's bucket model maps onto
  SQL tables.

Three less-conventional alternatives are also
worth a serious look. They sit alongside the six
above rather than replacing them; see
[aggregation-use-cases.md](aggregation-use-cases.md)
for the deeper exploration:

- **Stateless-fast (`fzf` / `ripgrep` style).**
  Re-read every run, but make the read fast
  enough that no cache is needed. ripgrep
  searches millions of lines per second; an FM
  + body parser tuned the same way could put
  cold-start under one second on tens of
  thousands of files.
- **Content-addressed cache (`git`-style
  objects).** A loose-objects directory of
  hashed FM blobs. No staleness check at all:
  the hash is the cache key. Faster than mtime
  but slower than mmap.
- **Tiered: fast cold start + opt-in warm
  daemon.** Default to stateless. A
  `mdsmith watch` daemon (P-2) adds warm
  caching only for sessions that elect it.

## In-flight items

| ID  | Plan / PR          | Notes                                         |
|-----|--------------------|-----------------------------------------------|
| H-1 | plan 122           | Hover for rule and directive docs             |
| H-2 | plan 131 (PR #238) | Symbol navigation, references, call hierarchy |

## Sequencing observations

Where the gaps cluster suggests a few natural
groupings rather than a fixed roadmap. None of
these are commitments; each waits for the trigger
named in its mini-plan.

- **Quick ergonomic wins.** L-1, L-4, M-1, C-6,
  Q-1, Q-2 are all small in effort and respond
  to triggers a single user request can satisfy.
  They unblock larger work without committing to
  it.
- **Schema ergonomics cluster.** S-1 unlocks S-2,
  S-3, S-4, S-6 by making inline schemas the
  primary surface. If schema work happens at all,
  it tends to start here.
- **The link-graph cluster.** L-1, L-4, L-3 / C-4
  all read or write the same workspace link
  graph. Doing them together reuses the parsing
  and indexing investment.
- **The CRUD cluster.** C-1 makes C-2, C-3, C-5,
  S-5, V-1 reachable. Without C-1 the others
  have no write moment to attach to. Whether to
  enter the cluster at all depends on whether
  mdsmith should grow a write surface; that is a
  bigger product question than any single gap.
- **The index cluster.** P-1, P-2, Q-5, Q-6, L-5
  all become more attractive together. The
  trigger for one is often the trigger for
  several; the [aggregation use-cases
  doc](aggregation-use-cases.md) walks the
  combined trade-off.

## What this exercise revealed

Three observations fall out of working through
the gaps systematically. They are descriptive,
not prescriptive — each leaves room for the
direction to change as triggers fire.

1. **Most "missing" features are write-side or
   cache-side.** Cache, watch, CRUD, migrations
   each trade some of mdsmith's determinism for
   capability. None are wrong; each carries a
   visible trigger and a visible cost.
2. **Schemas are closer than the surface
   suggests.** Both mdsmith and mdbase use
   Markdown files with declarative front matter.
   The differences (CUE vs DSL, body structure
   check vs body-as-docs) are real but smaller
   than they look. S-1 + S-2 + S-3 close most of
   the ergonomic gap if the trigger ever fires.
3. **The link graph is mdsmith's biggest
   underused asset.** MDS027 already builds it.
   Backlinks (L-4), wikilinks (L-1), and rename
   refactor (L-3) all flow from promoting that
   graph to a first-class subsystem.
