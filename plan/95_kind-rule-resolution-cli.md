---
id: 95
title: Kind/rule resolution observability via `kinds` subcommand
status: "🔲"
model: opus
summary: >-
  Replace `mdsmith config kinds/show/why` with a
  top-level `kinds` subcommand parallel to `archetypes`
  (which is being removed by plan 98). Adds per-leaf
  provenance, `check --explain`, and first-class
  `--json` output.
---
# Kind/rule resolution observability via `kinds` subcommand

## Goal

Make it easy to answer "why is this rule applied this
way to this file?". The CLI surface is a top-level
`kinds` subcommand that exposes:

- declared kinds and their merged bodies,
- the resolved kind list and merged rule config for a
  file (with provenance per leaf setting),
- the full merge chain for a single rule on a single
  file,
- a `--json` form of each, and
- a `check --explain` flag that attaches the same
  provenance trailer to each diagnostic.

`kinds` is parallel in shape to today's `archetypes`
subcommand, which plan 98 removes. Together the two
plans collapse overlapping concepts into one.

## Background

Plan 92 introduces kinds; plan 93 layers per-rule
`placeholders:` settings; plan 96 starts applying
them. As the rule config grows from "global +
overrides" to "global + kinds + assignment +
overrides", a flagged file's effective config becomes
hard to reproduce by reading `.mdsmith.yml` alone.
Provenance makes this debuggable.

## Design

### Per-leaf provenance

For every effective rule setting on a given file,
mdsmith tracks the chain of layers that produced its
final value. Provenance is **per leaf**, not per
rule-block: every individual scalar/list field is
tagged separately. Layers:

- `default` — the rule's built-in default
- `kinds.<name>` — set by the named kind's body
- `overrides[i]` — set by the i-th override entry
- `front-matter override` — set by the file's own
  front matter

Per-leaf granularity matters because plan 97 (deep-
merge) makes nested settings come from different
layers; the data model is the same shape now and then.
Today, under block-replace, every leaf in a rule's
final config carries the same source — that's a valid
output of the per-leaf model, not a special case.

### Subcommands

```text
mdsmith kinds list [--json]
  Print declared kinds with their merged bodies.

mdsmith kinds show <name> [--json]
  Print the merged body of one kind (replaces
  archetypes show).

mdsmith kinds path <name>
  Print the filesystem path of the kind's
  required-structure.schema, if any (replaces
  archetypes path).

mdsmith kinds resolve <file> [--json]
  Print the resolved kind list and merged rule
  config for a file, with provenance per leaf.

mdsmith kinds why <file> <rule> [--json]
  Print the full merge chain for one rule on one
  file: every layer, including no-ops, with the
  value at each step.
```

### `--explain` on `check` and `fix`

```text
$ mdsmith check --explain plan/92_…md
plan/92_…md:11:1 MDS022 file too long (305 > 300)
  └─ max-file-length.max=300 (default); kind 'plan'
     did not override
```

Trailer per diagnostic, scoped to the rule that
fired. Same provenance source as `kinds resolve`.

### JSON output

`--json` on every subcommand and on `check --explain`
emits a structured form. Schema is stable enough for
an LSP / VS Code extension to consume. Fields cover
file path, effective kind list (with sources), and
per-leaf settings with their merge chains.

## Tasks

1. Add a per-leaf provenance tracker to the config-
   merge pipeline. Each leaf setting's final value
   carries a list `[{layer, value, source}]`.
2. Add `mdsmith kinds list` (text + `--json`).
3. Add `mdsmith kinds show <name>` and
   `mdsmith kinds path <name>`.
4. Add `mdsmith kinds resolve <file>` rendering the
   resolved kind list and per-leaf provenance
   summary; add `--json`.
5. Add `mdsmith kinds why <file> <rule>` rendering
   the full merge chain for a single rule; add
   `--json`.
6. Add `--explain` flag to `check` and `fix`. After
   each diagnostic, print a one-line trailer naming
   the rule and the winning source of the setting
   that triggered the flag; in `--json` output the
   diagnostic carries an `explanation` object.
7. Document the JSON schema briefly in
   `docs/reference/cli.md`.
8. `mdsmith help kinds-cli` prints the subcommand
   summary.

## Acceptance Criteria

- [ ] `mdsmith kinds list` prints declared kinds
      with their merged bodies.
- [ ] `mdsmith kinds show <name>` prints one kind's
      merged body; exits 2 on unknown name.
- [ ] `mdsmith kinds path <name>` prints the path
      of the kind's `required-structure.schema:` if
      set; exits 2 otherwise.
- [ ] `mdsmith kinds resolve <file>` prints the
      resolved kind list and merged rule config;
      every leaf is tagged with its source (default
      / kind name / override / front-matter)
      (covered by test).
- [ ] `mdsmith kinds why <file> <rule>` prints the
      full merge chain — every layer that did or
      did not touch each leaf — for a single rule
      on a single file (covered by test).
- [ ] `mdsmith check --explain` prints, after each
      diagnostic, a trailer naming the rule and the
      source of the setting that triggered it
      (covered by test).
- [ ] `--json` on each `kinds` subcommand and on
      `check --explain` produces a stable structured
      form documented in `docs/reference/cli.md`
      (schema regression test).
- [ ] No new state is required of rule
      implementations; provenance lives in the merge
      pipeline only.
- [ ] `mdsmith help kinds-cli` summarizes the
      subcommand surface.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
