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
write a glob that doesn't match it. The plan entry above
uses `plan/[0-9]*_*.md` — the leading-digit pattern
matches every numbered plan file but skips `proto.md`,
which is then handled separately.

When a file should belong to two kinds, two entries can
match it. The order of those entries fixes the merge
order — kinds picked up earlier appear earlier in the
effective list.

In the example above, `plan/proto.md` matches both
`**/proto.md` (proto kind) and the explicit `plan/proto.md`
entry (plan kind), so its effective kind list is
`[proto, plan]`. A regular plan file like
`plan/96_kinds-adoption-and-docs.md` only matches the
`plan/[0-9]*_*.md` glob, so it resolves to `[plan]`.

## Merge order

Rule settings come from four layers, applied in this
order from lowest to highest precedence:

1. The rule's built-in defaults.
2. Top-level `rules:` defaults.
3. Each kind in the file's effective kind list, in order.
4. Each `overrides:` entry whose `files:` glob matches,
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
   every entry whose `files:` glob matches the file.
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
