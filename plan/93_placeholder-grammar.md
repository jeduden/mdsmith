---
id: 93
title: Placeholder grammar — opt-in token vocabulary
status: "✅"
model: sonnet
summary: >-
  Lift the ad-hoc placeholder escapes (`# ?`, `## ...`,
  `{var}`, CUE front-matter values) into one named
  vocabulary that rules consult through a shared
  `placeholders:` Configurable setting.
---
# Placeholder grammar — opt-in token vocabulary

## Goal

Make placeholder tokens (`# ?`, `## ...`, `{var}`,
CUE-pattern front-matter values) a first-class, named
vocabulary that any rule can opt into. A rule with the
`placeholders:` setting consults a shared helper to
treat configured tokens as opaque, not as content
violations.

This unblocks the adoption work in plan 96, which
needs the seven affected rules to stop tripping on
placeholder text in `proto.md` and similar files.

## Background

Files like `plan/proto.md` hold CUE schema patterns
in their front matter. Their bodies hold template
placeholders. Multiple rules flag those patterns as
content violations today:

- `first-line-heading` flags `# ?`
- `cross-file-reference-integrity` flags
  `[NAME](../../../docs/background/concepts/NAME.md)`
- `paragraph-readability`, `paragraph-structure` flag
  the placeholder paragraphs
- front-matter validation rejects CUE syntax

The current escape is `ignore:`, which silences every
rule. A scoped escape lets rules apply where they make
sense and skip the placeholder text where it doesn't.

## Design

### Token vocabulary

Initial named tokens, registered in a central helper:

- `var-token` — `{identifier}` interpolation
- `heading-question` — `# ?`, `## ?`, `### ?`
- `placeholder-section` — `## ...`
- `cue-frontmatter` — CUE pattern values in front
  matter (string predicates, regex literals,
  disjunctions)

The vocabulary is closed code — adding a token is one
helper change plus per-rule opt-ins. The set is not
tied to any specific kind.

### Per-rule `placeholders:` setting

Each opt-in rule exposes a `placeholders:` setting
through the existing `Configurable` interface. The
value is a list of token names. The rule treats those
tokens as opaque:

```yaml
kinds:
  proto:
    rules:
      first-line-heading:
        placeholders: [var-token, heading-question]
      cross-file-reference-integrity:
        placeholders: [var-token]
```

The shared helper provides:

- detection (does this AST node match a configured
  token?), and
- masking (rewrite the node to a content-neutral form
  for the rule's own analysis).

Each rule decides how to use the result. No rule
references token names hardcoded in its own logic —
the list comes from config.

### Components that opt in

- Rules: `first-line-heading`, `heading-increment`,
  `no-emphasis-as-heading`,
  `cross-file-reference-integrity`,
  `paragraph-readability`, `paragraph-structure`,
  `required-structure` (for front-matter schema
  checks).
- Directive consumers: `catalog` front-matter
  interpolation.
- Engine: front-matter parsing under the
  `front-matter:` config key. The same parser feeds
  the `query` subcommand, so `query` honors the
  vocabulary automatically — without this, a file
  with CUE-pattern front matter (e.g. `proto.md`)
  fails to parse and breaks `query`.

## Tasks

1. Add the placeholder-grammar helper to a shared
   internal package: token registry, detection API,
   masking API.
2. Add a `placeholders:` setting to each opt-in rule
   via `Configurable`; default empty list.
3. Wire each rule's analysis to consult the helper
   when its `placeholders:` list is non-empty.
4. Wire `catalog` interpolation and engine
   front-matter parsing to the helper.
5. Document the placeholder grammar as a concept
   page at
   `docs/background/concepts/placeholder-grammar.md`
   (the `archetypes` doc directory is renamed in
   plan 98). Describe the token vocabulary, the
   `placeholders:` rule-setting contract, and how
   rules opt in. Link from each opt-in rule README.
6. `mdsmith help placeholder-grammar` prints a short
   concept page.
7. Unit tests per rule: with `placeholders:` set,
   placeholder tokens produce no diagnostics; with
   `placeholders:` empty, current behavior is
   unchanged.
8. `query` regression: a file in a placeholder-aware
   kind whose front matter contains CUE patterns
   parses successfully and is selectable by `query`.

## Acceptance Criteria

- [x] The helper recognizes the four initial tokens
      and exposes detection + masking APIs.
- [x] Each opt-in rule reads its `placeholders:`
      setting via `Configurable` and consults the
      helper (verified by per-rule unit test).
- [x] With `placeholders:` empty, every rule produces
      the same diagnostics it does today (regression
      test).
- [x] With `placeholders:` set, configured tokens
      produce no diagnostics from that rule.
- [x] `catalog` front-matter interpolation and
      engine front-matter parsing honor the same
      vocabulary (test covers a CUE-pattern value in
      a `<?catalog?>`-eligible file).
- [x] Adding a new token is a one-file change in the
      helper plus per-rule opt-ins; no rule names
      tokens hardcoded in its logic (enforced by
      review).
- [x] Concept page at
      `docs/background/concepts/placeholder-grammar.md`
      describes the contract and is linked from each
      opt-in rule README.
- [x] `mdsmith help placeholder-grammar` prints the
      concept page summary.
- [x] `mdsmith query` parses a file with CUE-pattern
      front matter under a placeholder-aware kind
      (covered by test).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
