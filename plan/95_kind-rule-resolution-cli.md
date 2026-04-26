---
id: 95
title: Kind/rule resolution observability CLI
status: "🔲"
summary: >-
  CLI surface for troubleshooting which kinds and rule
  settings apply to a file: `config kinds`,
  `config show <file>` with provenance,
  `config why <file> <rule>`, `check --explain`,
  and `--json` output for tooling.
---
# Kind/rule resolution observability CLI

## Goal

Make it easy to answer "why is this rule applied this
way to this file?". The CLI surface covers four
shapes of the same underlying provenance data:

- **List**: which kinds are declared, with their merged
  bodies.
- **Per file**: the resolved kind list and the merged
  rule config, each setting tagged with its source.
- **Per file + rule**: the full merge chain for one
  rule on one file (every layer, including no-ops).
- **Inline with diagnostics**: `check --explain`
  attaches a trailer to each finding showing which
  setting and source produced it.

All four share the same JSON output for tooling /
LSP / IDE consumption.

## Background

Plan 92 introduces kinds; plan 93 layers per-rule
`placeholders:` settings; future plans (e.g. 96) start
applying them. As the rule config grows from "global +
overrides" to "global + kinds + assignment + overrides",
a flagged file's effective config becomes harder to
reproduce by reading `.mdsmith.yml` alone. Provenance
makes this debuggable.

## Design

### Provenance data model

For every effective rule setting on a given file,
mdsmith tracks the chain of layers that produced its
final value:

- `default` — the rule's built-in default
- `kinds.<name>` — set by the named kind's body
- `overrides[i]` — set by the i-th override entry
- `front-matter override` — set by the file's own
  front matter

The chain is ordered (lowest-precedence first). The
final value comes from the latest layer that touched
the setting. `mdsmith config show` and friends render
this chain.

### Subcommands

```text
mdsmith config kinds [--json]
  Lists declared kinds with their merged bodies.

mdsmith config show <file> [--json]
  Resolved kind list and merged rule config for the
  file, each setting tagged with its winning source.

mdsmith config why <file> <rule> [--json]
  Full merge chain for one rule on one file: every
  layer, including no-ops, with the value at each
  step.
```

### `--explain` on `check` and `fix`

```text
$ mdsmith check --explain plan/92_…md
plan/92_…md:11:1 MDS022 file too long (305 > 300)
  └─ max-file-length.max=300 (default); kind 'plan'
     did not override
```

Trailer per diagnostic, scoped to the rule that
fired. Same provenance source as `config show`.

### JSON output

`--json` on every subcommand and on `check --explain`
emits a structured form. Schema is stable enough for
an LSP / VS Code extension to consume. Fields cover
file path, effective kind list (with sources), and
per-rule settings with their merge chains.

## Tasks

1. Add a provenance tracker to the config-merge
   pipeline. Each setting's final value carries a
   list of layer entries: `{layer, value, source}`.
2. Add `mdsmith config kinds` (text + `--json`).
3. Add `mdsmith config show <file>` rendering the
   resolved kind list and per-setting provenance
   summary; add `--json`.
4. Add `mdsmith config why <file> <rule>` rendering
   the full merge chain for a single rule; add
   `--json`.
5. Add `--explain` flag to `check` and `fix`. After
   each diagnostic, print a one-line trailer naming
   the rule and the winning source of the setting
   that triggered the flag; add `--json` (diagnostic
   carries an `explanation` object).
6. Document the JSON schema briefly in
   `docs/reference/cli.md`.

## Acceptance Criteria

- [ ] `mdsmith config kinds` lists declared kinds
      with their merged bodies.
- [ ] `mdsmith config show <file>` prints the
      resolved kind list and merged rule config; each
      setting is tagged with its source (default /
      kind name / override / front-matter override)
      (covered by test).
- [ ] `mdsmith config why <file> <rule>` prints the
      full merge chain — every layer that did or did
      not touch the setting — for a single rule on a
      single file (covered by test).
- [ ] `mdsmith check --explain` prints, after each
      diagnostic, a trailer naming the rule and the
      source of the setting that triggered it
      (covered by test).
- [ ] `--json` on each subcommand and on
      `check --explain` produces a stable structured
      form documented in `docs/reference/cli.md`
      (schema regression test).
- [ ] No new state is required of the rule
      implementations; provenance lives in the merge
      pipeline only.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
