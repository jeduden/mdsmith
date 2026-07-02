---
id: 2607022118
title: General occurrence rule — bound how often a pattern appears per scope
status: "🔲"
summary: >-
  Add one `occurrence` rule that bounds how many times a
  token or pattern may appear within a scope (file, section,
  or paragraph). It subsumes an em-dash-per-paragraph cap and
  a term-density check as presets, and reads its match set
  from named `lists:`. Counting only — flag, do not rewrite.
model: sonnet
depends-on: []
---
# General occurrence rule — bound how often a pattern appears per scope

## Goal

Give mdsmith one deterministic counting rule. It fails when a
token or pattern appears too often, or too rarely, in a scope.
An over-used em dash or a repeated pet phrase is then caught
mechanically, not by an LLM.

## Context

mdsmith's prose-tell family is binary today. `forbidden-text`
(MDS056) and `forbidden-paragraph-starts` (MDS055) fire on
the first occurrence; `required-mentions` (MDS058) fires when
a term is absent. None of them can say "at most three em
dashes per paragraph" or "no single listed buzzword more than
twice per section". There is no em-dash rule at all.

Vale covers this ground with one general
[`occurrence`](https://vale.sh/docs/checks/occurrence) check:
a token, a scope, and a `min`/`max` bound. That single
primitive expresses a paragraph em-dash cap, a sentence-length
limit, and a per-section keyword ceiling. mdsmith re-implements
a slice of it per rule and stops short of the general case.

This plan adds the general primitive once and expresses the
specific checks as presets over it. It builds on the named
word-list mechanism (`lists:`, plan 2606251522 / PR #694): the
match set for term-density comes from a shared list, not a
per-rule literal.

## Design

A new rule, `occurrence`, reclaims the unused **MDS060** slot.
It counts matches of a pattern set within a scope. It fails
when the count leaves the configured band.

Settings:

- `scope`: `file` | `section` | `paragraph` (default
  `paragraph`). A section is a heading and the blocks under
  it; matching the scope model MDS058 already walks.
- `tokens`: a list of literal strings to count. This is the
  `WordlistTarget()` key, so `lists:` entries union into it.
- `pattern`: an alternative single Go RE2 pattern, compiled at
  package scope (mutually exclusive with `tokens`).
- `min` / `max`: inclusive bounds on the per-scope count.
  Omitted `min` is 0; omitted `max` is unbounded.
- `count`: `each` (bound each token separately, the default)
  or `combined` (bound the sum across all tokens). An em-dash
  cap uses `combined`; a per-buzzword ceiling uses `each`.
- `case-sensitive`: default false.

Matching folds inline-code spans and code blocks out first,
walking `f.AST`, so a fenced example never trips the count.
The diagnostic names the token, the scope, the observed
count, and the bound.

Presets (documented; a user pins them by name via the
convention/rule config, no new Go per preset):

- `em-dash-density`: `pattern: "—"`, `scope: paragraph`,
  `count: combined`, `max: 2`. The motivating case — a run
  of em dashes per paragraph is a strong machine tell.
- `term-density`: `scope: section`, `count: each`, `max: 2`,
  `lists: [<buzzwords>]`. Caps how often any one listed term
  repeats in a section, so a curated buzzword list can be
  "banned outright" (MDS056) or merely "rationed" (this rule).

The rule counts; it does not rewrite. A count is not
mechanically fixable — cutting the third em dash or the fourth
"leverage" is an authoring choice. Auto-fix for word choice is
the separate substitution rule (plan 2607022120).

Allocation budget: the pattern compiles at package scope. The
scope walk reuses a loop-local counter. So `Check` stays within
the ≤10-alloc budget on representative input.

## Tasks

1. Add package `internal/rules/occurrence`: the `Rule`, its
   `ApplySettings` (scope, tokens, pattern, min, max, count,
   case-sensitive), and `Check`. Red/green per setting.
2. Implement `WordlistTarget() string { return "tokens" }`
   plus the `rule.WordlistConsumer` assertion, so `lists:`
   unions into `tokens`.
3. Register the rule as MDS060 and add
   `internal/rules/MDS060-occurrence/` with README and
   `good/`/`bad/` fixtures, including a `lists:`-driven case
   and an `em-dash-density` case.
4. Add the `em-dash-density` and `term-density` presets to the
   convention layer and document that a project pins them by
   name; verify the alloc-budget test covers the rule.
5. Add `docs/reference/cli` / rules docs entries and a
   `docs/guides/metrics-tradeoffs.md` note on choosing a
   density band; regenerate catalogs with `mdsmith fix`.

## Acceptance Criteria

- [ ] A paragraph with three em dashes fails
      `em-dash-density`; two pass.
- [ ] A section repeating a listed term three times fails
      `term-density` with `max: 2`; the same term in a fenced
      code block does not count.
- [ ] `lists:` entries union into `tokens`, proven by a
      fixture whose match set lives in `.mdsmith/wordlists/`.
- [ ] `min` catches an under-count (a required term missing
      from a scope) with a clear message.
- [ ] The rule's `Check` stays within the alloc budget
      (`internal/integration/alloc_budget_test.go`).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
