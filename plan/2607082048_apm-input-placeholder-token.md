---
id: 2607082048
title: "Placeholder token for APM `${input:name}` prompt parameters"
status: "✅"
model: sonnet
summary: >-
  Add one token to the closed placeholder vocabulary that matches
  APM `${input:NAME}` prompt parameters, so MDS023, MDS024, MDS018,
  and MDS004 treat them as opaque instead of prose. Opportunity A-3
  in the APM research.
depends-on: []
---
# Placeholder token for APM `${input:name}`

## Goal

Let a rule treat APM's `${input:name}` prompt
parameters as opaque, the same way
[`var-token`](../docs/background/concepts/placeholder-grammar.md)
already handles `{identifier}` interpolations. An
APM `.prompt.md` body dense with `${input:pr_url}`
tokens should not trip the prose rules on text that
is a template variable, not content.

## Background

APM `.prompt.md` bodies interpolate `${input:NAME}`
tokens; `apm run` substitutes `--param` values into
them, and `apm install` rewrites them per harness
(kept verbatim for Copilot, rewritten to `$name` for
Claude). The input-name grammar APM documents is
`[A-Za-z][\w-]{0,63}`.

mdsmith's placeholder vocabulary is a closed set. The
`var-token` matcher recognizes only `{identifier}`
forms, so `${input:pr_url}` is opaque to nothing:
[MDS023](../internal/rules/MDS023-paragraph-readability/README.md),
[MDS024](../internal/rules/MDS024-paragraph-structure/README.md),
[MDS018](../internal/rules/MDS018-no-emphasis-as-heading/README.md),
and
[MDS004](../internal/rules/MDS004-first-line-heading/README.md)
all read the token as prose. This is opportunity A-3
in the
[APM opportunity catalogue](../docs/research/apm-mdsmith/opportunities.md).

## Non-Goals

- Declaration/usage cross-checking (an undeclared or
  case-mismatched `${input:Scope}`). That is a
  separate rule, opportunity A-6.
- The post-Claude `$name` bare form. This plan covers
  the source `${input:NAME}` form only.
- Shipping the full APM kind pack. That is
  [plan 2607082050](2607082050_apm-coexist-guide-and-kind-pack.md);
  this token is a dependency it names.

## Tasks

1. Red/green: unit tests in
   [`internal/placeholders`](../internal/placeholders/)
   for `ContainsBodyToken` and `MaskBodyTokens` on
   `${input:pr_url}`, `${input:focus-area}`, and a
   non-matching `${output:x}`.
2. Add the token constant and its matcher to the
   closed vocabulary: shape `${input:NAME}` with
   `NAME` matching `[A-Za-z][\w-]{0,63}`. Compile the
   pattern at package scope per the allocation budget.
3. Wire the token into `ContainsBodyToken` (skip
   check) and `MaskBodyTokens` (mask to a neutral
   word) so readability and structure rules score the
   non-placeholder text.
4. Document the token in
   [placeholder-grammar.md](../docs/background/concepts/placeholder-grammar.md):
   add the vocabulary-table row and the opt-in rule
   list.
5. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [x] A paragraph whose only non-prose content is
      `${input:pr_url}` produces no MDS023 or MDS024
      diagnostic when the token is configured.
- [x] The token name appears in the placeholder
      vocabulary table and the opt-in rule list.
- [x] The matcher rejects `${notinput:x}` and
      `${input:bad name}` (space is outside the
      grammar).
- [x] `Check` stays within the allocation budget
      (regexp compiled at package scope).
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint
      run` reports no issues.
- [x] `mdsmith check .` — 0 failures.
