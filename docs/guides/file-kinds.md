---
title: File Kinds
weight: 20
summary: >-
  How to declare file kinds, assign files to them, and read
  the merged rule config that results.
---
# File Kinds

A **kind** is a named bundle of rule settings that mdsmith
applies to a set of files. Kinds let you share per-rule
tuning across files that serve the same purpose — schema
templates, plan documents, rule READMEs, prompts, security
notes — without copying the same overrides into every glob
that matches them.

mdsmith ships **no built-in kinds**. Each project picks the
names that fit its repository.

## Declaring a kind

Kinds live under the `kinds:` key in `.mdsmith.yml`. Each
kind body has the same shape as an entry under
`overrides:`, minus `glob:` — files are bound to kinds
separately.

```yaml
kinds:
  plan:
    rules:
      required-structure:
        schema: plan/proto.md
      paragraph-readability: false
  proto:
    rules:
      first-line-heading: false
      paragraph-readability: false
```

A kind that sets `rules.required-structure.schema:`
attaches that CUE schema to every file of the kind. A kind
that sets `rule-name: false` disables the rule for every
file of the kind.

Referencing an undeclared kind name from front matter or
`kind-assignment:` is a config error.

### Validating the file path

A kind can declare a `path-pattern:` glob that the
workspace-relative path of every file in the kind must
match. Use it to enforce filename conventions —
plan-ID prefixes, RFC numbering, runbook slugs —
without a custom CI script.

```yaml
kinds:
  plan:
    path-pattern: "plan/[0-9][0-9]*_*.md"
    rules:
      required-structure:
        schema: plan/proto.md
  rfc:
    path-pattern: "docs/rfc/RFC-[0-9][0-9][0-9][0-9].md"
```

A file whose path does not match the kind's pattern
produces an MDS020 diagnostic anchored to line 1 of the
file. For `plan/early-draft.md` with the `plan` kind
above, the diagnostic reads:

```text
filename: got "plan/early-draft.md", expected
  glob plan/[0-9][0-9]*_*.md
schema: kinds[plan] / path-pattern
```

The pattern uses the same doublestar syntax as
`overrides:`, `ignore:`, and `kind-assignment:`
(see [Glob patterns](../reference/globs.md)). Because
glob syntax has no "exactly digits" character class,
the pattern is an approximation: `[0-9][0-9]*` enforces a
two-digit prefix plus any trailing characters, not strict
integer-only. For tighter constraints, combine
`path-pattern:` with a `<?require filename:?>` directive
on the schema — both run, and each emits its own
diagnostic when violated.

`mdsmith kinds show <name>` prints `path-pattern:`
alongside the kind's rule settings when it's set, so the
constraint is auditable from one command.

## Assigning files to kinds

A file's effective kind list is built from two sources,
concatenated in this order:

1. The file's own front-matter `kinds:` field (a YAML
   list).
2. Matching entries in `kind-assignment:`, in the order
   they appear in the config; within an entry, kinds in
   the order listed. An entry can select files by `glob:`
   and/or `fields-present:` (front-matter keys with
   non-null values).

Duplicate names are dropped after their first occurrence.

### Front-matter assignment

A file can declare its kinds inline. Use this for one-off
files where a glob doesn't make sense.

```markdown
---
kinds: [plan]
id: 92
status: 🔲
---
# File kinds
```

A multi-kind file uses a multi-element list:
`kinds: [draft, worksheet]`. Merge order matches list
order.

### Glob assignment

`kind-assignment:` is a list of entries. Each entry has
`glob:` (the same doublestar pattern syntax as `overrides:`
and `ignore:`) and `kinds:` (the names to apply).

```yaml
kind-assignment:
  - glob: ["**/proto.md"]
    kinds: [proto]
  - glob: ["plan/*.md"]
    kinds: [plan]
  - glob: ["internal/rules/proto.md",
           "internal/rules/MDS*/README.md"]
    kinds: [rule-readme]
```

Globs use the same matcher as `overrides:` and `ignore:`
(see [Glob patterns](../reference/globs.md)). The plan
entry uses `plan/*.md`, which matches every plan file
including `proto.md`. The directory glob naturally
includes `proto.md`, and that is what we want here: the
proto kind disables structural rules on `proto.md`, while
the plan kind supplies `required-structure.schema:` so
the file is recognized as its own schema.

Where the directory glob targets a different filename
(for example `internal/rules/MDS*/README.md`, which does
not match `internal/rules/proto.md`), list the schema
file explicitly alongside the convention glob.

A `!`-prefix on a pattern re-excludes a path. Use it in
`overrides:` to keep content-tuning settings off
`proto.md` (the proto kind already handles those
files). Avoid `!`-exclusion in `kind-assignment:` for a
schema kind — excluding `proto.md` there would also
strip the `required-structure.schema:` that marks the
file as its own schema.

When a file should belong to two kinds, two entries can
match it. The order of those entries fixes the merge
order — kinds picked up earlier appear earlier in the
effective list.

In the example above, `plan/proto.md` matches both
`**/proto.md` (proto kind) and `plan/*.md` (plan kind),
so its effective kind list is `[proto, plan]`. A regular
plan file like `plan/96_kinds-adoption-and-docs.md` only
matches `plan/*.md`, so it resolves to `[plan]`.

### Field-presence assignment

An entry can require that a file's front matter carries a
configured set of keys, each with a non-null value. Pair
this with `glob:` when a project identifies a file's role
by its front-matter shape rather than (or in addition to)
its location.

```yaml
kind-assignment:
  - glob: ["docs/**"]
    kinds: [doc]
  - fields-present: [status, priority, assignee]
    kinds: [task]
  - glob: ["plan/*.md"]
    fields-present: [id]
    kinds: [plan]
```

Within a single entry, `glob:` and `fields-present:`
combine with **AND** — every selector that is set must
match. Across entries, matches union (OR), the same as
for glob-only entries.

A field is "present" when the key appears in front matter
with a non-null value. A key set to YAML `null` (e.g.
`status: null`) does **not** count: the user wrote the
key but did not fill it.

For the config above, a file at
`anywhere/important.md` whose front matter is:

```markdown
---
status: open
priority: high
assignee: alice
---
```

resolves to `[task]` because the second entry's
`fields-present:` selector is satisfied. A file at
`plan/132_inline.md` with `id: 132` in its front matter
resolves to `[plan]` — the third entry's `glob:` matches
the path **and** its `fields-present:` finds a non-null
`id`.

`mdsmith kinds resolve <file>` names the matching entry
index and the selectors that fired, so you can confirm
which rule did the work:

```text
file: plan/132_inline.md
effective kinds:
  - plan (from kind-assignment[2]: glob plan/*.md AND fields-present id)
```

## Schema inheritance with `extends`

A kind can build on another kind's schema via the `extends:` key.
Inheritance is single-parent — a child names exactly one parent —
and the engine combines them under CUE refinement: the child's
constraints unify with the parent's, so any value the child
accepts must also satisfy the parent. This keeps a base schema
authoritative while letting child variants add or narrow
constraints.

```yaml
kinds:
  rfc-base:
    schema:
      frontmatter:
        id: '=~"^RFC-[0-9]{4}$"'
        authors: '[...string] & [_, ...string]'
        created: date

  rfc-ratified:
    extends: rfc-base
    schema:
      frontmatter:
        ratified-on: date
        status: '"ratified"'

  rfc-draft:
    extends: rfc-base
    schema:
      frontmatter:
        status: '"draft" | "in-review"'
```

`rfc-ratified` inherits `id`, `authors`, and `created` from
`rfc-base`, adds `ratified-on`, and locks `status` to a single
literal. `rfc-draft` inherits the same base and declares its own
`status` disjunction. A file resolving to `rfc-ratified` must
satisfy every constraint from both layers; a child that
re-declares a parent key joins them via CUE `&`, so a narrowing
expression refines the parent and an incompatible one is
rejected at config load.

### Conflict semantics

`frontmatter:` keys unify under CUE's standard rules:

| Parent expression    | Child expression | Effective                              |
|----------------------|------------------|----------------------------------------|
| `"open" \| "closed"` | `"open"`         | `"open"` (refinement; OK)              |
| `"open" \| "closed"` | `"ratified"`     | conflict — no value satisfies          |
| `int`                | `string`         | conflict — no overlap                  |
| `string & !=""`      | `=~"^[A-Z]"`     | non-empty string starting with capital |

A conflict surfaces at config load with both layer names. A
narrowing child is the supported pattern: keep one base kind and
let variants tighten the constraint.

### Sections replace

`sections:` does **not** unify — heading templates compose by
sequence, not by constraint, so a child that declares its own
`sections:` list wholly replaces the parent's. To extend a
parent's template, copy the parent's lines and add to them. A
child that declares no `sections:` inherits the parent's tree
verbatim.

### File-based schemas

A `proto.md` file declares its parent in front matter:

```markdown
---
extends: rfc-base.proto.md
ratified-on: date
status: '"ratified"'
---
# {id}

## Decision
```

The path is resolved relative to the schema file (the same rule
`<?include?>` follows). Absolute paths and `..` traversal are
rejected. Inline-kind `extends:` cycles surface at config load;
file-schema cycles surface when MDS020 first parses the schema
during `check` or `fix`. Both forms name the full cycle path in
the diagnostic.

### Deprecating a frontmatter field

A schema can mark a frontmatter field deprecated so
tooling can route the warning separately from a hard
schema violation. The diagnostic surfaces with Warning
severity (LSP shows it as a warning squiggle, JSON
output sets `"severity": "warning"`) while a project
migrates away from the field. `mdsmith check` today
exits non-zero on any diagnostic regardless of
severity; a future `--error-on` flag would let CI
distinguish warnings from errors.

The field's value becomes a mapping carrying `type:`
plus the metadata:

```yaml
kinds:
  plan:
    schema:
      frontmatter:
        legacy_owner:
          type: string
          deprecated: true
          message: 'use "owner" instead'
        owner:
          type: string
```

A document that still carries `legacy_owner:` then
reports a Warning-severity MDS020 diagnostic naming the
field and the message. The CUE constraint (`type:`) still
applies, so a value that violates the type also raises
the usual Error — the deprecation never silences other
checks. Use `replaced-by: <name>` in place of `message:`
for the canonical "deprecated field; replaced by `name`"
sentence; see the
[MDS020 README](../../internal/rules/MDS020-required-structure/README.md)
for the diagnostic shape and the precedence rules when
both are set.

**Reserved-key limitation.** A mapping with `type:` plus
the literal `deprecated: true` is always parsed as
metadata, not as a CUE struct constraint. To constrain
a frontmatter field whose value is a struct that
legitimately binds a `type` field, write the constraint
as a string CUE expression instead — e.g.
`config: '{type: "production" | "staging"}'`. The
`message:` and `replaced-by:` keys are reserved the
same way: setting either without `deprecated: true` is
treated as a typo and rejected at config load.

### Auditing the chain

`mdsmith kinds show <name>` prints the parent line and the
resolved frontmatter with per-field provenance, so you can see
which layer contributed each constraint without re-reading every
schema:

```text
rfc-ratified:
  extends: rfc-base
  extends-chain: rfc-ratified -> rfc-base
  rules: …
  effective-frontmatter:
    authors: [...string] & [_, ...string]                  # from rfc-base
    created: =~"^\d{4}-\d{2}-\d{2}$"                       # from rfc-base
    id: =~"^RFC-[0-9]{4}$"                                 # from rfc-base
    ratified-on: =~"^\d{4}-\d{2}-\d{2}$"                   # from rfc-ratified
    status: "ratified"                                     # from rfc-ratified
```

Bare-name shortcuts (`date`, `nonEmpty`, …) expand to their
canonical CUE in the printed output, so the audit shows the
constraint as the validator sees it rather than the shortcut
spelling.

Add `--json` for the structured form.

## Merge order

Rule settings come from four layers, applied in this
order from lowest to highest precedence:

1. The rule's built-in defaults.
2. Top-level `rules:` defaults.
3. Each kind in the file's effective kind list, in order.
4. Each `overrides:` entry whose `glob:` matches,
   in config order.

Across all four layers the config is **deep-merged**
rule by rule:

- **Maps** merge key by key — a setting from an earlier
  layer survives if the later layer doesn't touch the
  same key.
- **Scalar** values at a leaf replace the earlier value
  wholesale.
- **List** settings replace by default. A rule can opt
  a specific list key into append mode (the
  `placeholders:` setting is the canonical example).
- A **bool-only** entry such as `rule-name: false`
  toggles `enabled` without erasing any other settings
  the rule inherited from earlier layers.

Because the merge is key-by-key, a kind that sets one
key on a rule leaves the rule's other settings intact
from whichever earlier layer configured them.

## Conflict resolution

When two kinds in the effective list configure the same
rule, the **later kind wins** — its settings deep-merge
over the earlier kind's settings for that rule, with
later scalar values overwriting earlier ones key by key.
The same applies between kinds and overrides.

Order is list-driven, so the result is stable across
runs.

### Putting it together

For a file resolved as `[proto, plan]` with the kinds
above:

- `required-structure` comes from `plan` (the `proto`
  kind doesn't set it, so `plan`'s setting stands).
- `paragraph-readability: false` comes from `proto` (the
  `plan` kind doesn't touch it).
- `first-line-heading: false` comes from `proto` (the
  `plan` kind doesn't touch it).

If a glob override on `plan/*.md` then sets
`max-file-length: { max: 500 }`, that override applies on
top of the kinds and replaces only the
`max-file-length` rule.

## Troubleshooting

When a file produces an unexpected diagnostic — or the
diagnostic you expected doesn't fire — start with the
resolved kind list and the merged rule config for that
file.

`mdsmith kinds resolve <file>` prints both: the effective
kind list and the merged rule settings, with a per-leaf
source so you can see which layer set each value. Add
`--json` for a structured form. For a single rule's full
merge chain, use `mdsmith kinds why <file> <rule>`. To
attach the same source trailer to each diagnostic, run
`mdsmith check --explain` (or `fix --explain`).

If you'd rather walk the merge by hand, the same
information is recoverable by reading `.mdsmith.yml`
against the merge rules above:

1. Read the file's front matter for any `kinds:` field.
2. Walk `kind-assignment:` top to bottom and collect
   every entry whose selectors all match the file —
   `glob:` against the path and `fields-present:`
   against the file's front matter.
3. Concatenate the two lists, dropping duplicates after
   first occurrence — that's the effective kind list.
4. Apply built-in defaults, then the top-level `rules:`
   block, then each kind body in order, then each
   matching `overrides:` entry. Each layer deep-merges
   its settings over the accumulated config.

For a quick primer on the same model from the CLI, run
`mdsmith help kinds`.

## See also

- [Enforcing Document Structure with Schemas](directives/enforcing-structure.md)
  — how `required-structure` reads the schema attached by
  a kind.
- [Placeholder grammar](../background/concepts/placeholder-grammar.md)
  — opt-in tokens that let kinds keep template files
  green under the same rules used for content.
- [Schema field types](../reference/schema-types.md)
  — named shortcuts (`date`, `email`, `url`, …) for
  schema frontmatter values, so a kind's `schema:`
  block does not have to re-derive the same CUE regex
  every project lands on.
