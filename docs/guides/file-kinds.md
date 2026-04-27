---
title: File Kinds
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
`overrides:`, minus `files:` — files are bound to kinds
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

## Assigning files to kinds

A file's effective kind list is built from two sources,
concatenated in this order:

1. The file's own front-matter `kinds:` field (a YAML
   list).
2. Matching entries in `kind-assignment:`, in the order
   they appear in the config; within an entry, kinds in
   the order listed.

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
`files:` (the same glob shape as `overrides:` and
`ignore:`) and `kinds:` (the names to apply).

```yaml
kind-assignment:
  - files: ["**/proto.md"]
    kinds: [proto]
  - files: ["plan/proto.md", "plan/[0-9]*_*.md"]
    kinds: [plan]
  - files: [".claude/skills/proto.md",
            ".claude/skills/*/SKILL.md"]
    kinds: [skill]
```

Globs use the same matcher as `overrides:` and `ignore:`.
There is no `!`-negation; to exclude a narrower path,
write a glob that doesn't match it. `plan/[0-9]*_*.md`
naturally excludes `plan/proto.md`. To pick up the
narrower file with a different kind, list it explicitly
in another `kind-assignment:` entry — duplicate kinds
across entries are deduplicated, and the most specific
glob can carry an additional kind.

A file that should belong to two kinds (for example, the
schema template that is both a `proto` and a `plan` file)
appears in both globs:

```yaml
- files: ["**/proto.md"]
  kinds: [proto]
- files: ["plan/proto.md", "plan/[0-9]*_*.md"]
  kinds: [plan]
```

`plan/proto.md` resolves to `[proto, plan]`; `plan/96.md`
resolves to `[plan]`.

## Merge order

Rule settings come from four layers, applied in this
order from lowest to highest precedence:

1. Top-level `rules:` defaults.
2. Each kind in the file's effective kind list, in order.
3. Each `overrides:` entry whose `files:` glob matches,
   in config order.

Within each kind and each override, a rule mentioned in
the layer **replaces the rule's entire config block** —
nested settings do not deep-merge. This matches today's
`overrides:` semantics.

The block-replace contract has one consequence worth
spelling out: if a kind sets one setting on a rule, it
loses the rule's other settings. A kind that wants to add
a `placeholders:` token to a rule that already had
`level: 1` configured at the top level must restate
`level: 1` in the kind body, or the rule will reset to
its built-in default level.

## Conflict resolution

When two kinds in the effective list configure the same
rule, the **later kind wins** — its rule block replaces
the earlier kind's rule block in full. The same applies
between kinds and overrides: a matching `overrides:`
entry replaces whatever the kinds produced for that rule.

Order is list-driven, so the result is stable across
runs.

### Putting it together

For a file resolved as `[proto, plan]` with the kinds
above:

- `required-structure` comes from `plan` (later kind
  replaces `proto`'s body, even if `proto` had set the
  rule).
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

`mdsmith kinds resolve <file>` (introduced by the
`kinds` subcommand) prints both: the effective kind list
and the merged rule settings, with a per-leaf source so
you can see which layer set each value. Use it whenever
the rule config you read in `.mdsmith.yml` doesn't match
the diagnostics on a file.

Until that subcommand lands, the same information is
recoverable by reading `.mdsmith.yml` against the merge
rules above:

1. Read the file's front matter for any `kinds:` field.
2. Walk `kind-assignment:` top to bottom and collect
   every entry whose `files:` glob matches the file.
3. Concatenate the two lists, dropping duplicates after
   first occurrence — that's the effective kind list.
4. Apply each kind body in order, then each matching
   `overrides:` entry. Each layer replaces the rule
   block it touches.

For a quick primer on the same model from the CLI, run
`mdsmith help kinds`.

## See also

- [Enforcing Document Structure with Schemas](directives/enforcing-structure.md)
  — how `required-structure` reads the schema attached by
  a kind.
- [Placeholder grammar](../background/concepts/placeholder-grammar.md)
  — opt-in tokens that let kinds keep template files
  green under the same rules used for content.
