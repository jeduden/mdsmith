---
title: Coexist with Vale and remark
weight: 60
summary: >-
  Vale owns language-aware brand voice and style; remark owns
  Markdown AST transformations; mdsmith owns formatting,
  cross-file integrity, generated sections, and deterministic
  prose tells such as banned words, required terms, and
  proper-noun casing. Prose splits by whether a check is
  mechanical or language-aware.
---
# Coexist with Vale and remark

Vale, remark, and mdsmith solve different problems.
Most docs teams run two of them; a few run all three.
This page draws the boundary so each tool gets the
scope it is best at.

## Who owns what

The split is not only topic. It is also *how* a check works.
mdsmith runs deterministic, list-driven checks and can
auto-fix the mechanical ones. Vale runs language-aware checks
that weigh context and voice. Prose falls to whichever side a
given check sits on.

| Concern                                     | Owner   |
| ------------------------------------------- | ------- |
| Brand voice, tone, and register             | Vale    |
| Passive voice and hedging detection         | Vale    |
| Context-aware style-guide suggestions       | Vale    |
| Inclusive-language and jargon guidance      | Vale    |
| Markdown AST transformations                | remark  |
| Custom Markdown plugins                     | remark  |
| Whitespace, heading style, code fences      | mdsmith |
| Bare URLs, link reference integrity         | mdsmith |
| Generated sections (catalog, toc)           | mdsmith |
| Cross-file link and anchor integrity        | mdsmith |
| File kinds and per-directory schemas        | mdsmith |
| Banned words and phrases (literal denylist) | mdsmith |
| Proper-noun casing (fixed name set)         | mdsmith |
| Required terms per section                  | mdsmith |
| Readability budgets (ARI, sentence count)   | mdsmith |

Readability appears in both columns: Vale has
proselint-style readability rules, mdsmith has
`paragraph-readability` (ARI). Pick one as the source of
truth for that signal so writers do not see the same
warning twice.

### Deterministic versus language-aware

A check belongs to mdsmith when it is literal and repeatable:
a fixed word, a fixed phrase, a name cased one way, a term
that must appear in a section. `forbidden-text` and
`forbidden-paragraph-starts` flag banned words and openers.
`proper-names` flags miscased names. `required-mentions`
flags a missing required term. The `no-llm-tells` convention
bundles the common denylist. Each check fires on a literal
match, so its result is exact. These rules flag today; they
do not rewrite.

A check belongs to Vale when it needs judgment: is a sentence
passive, is a tone off-brand, would other phrasing read more
plainly. Those answers depend on context, not a lookup, so
they stay with Vale's language-aware engine.

The line moves as mdsmith adds deterministic prose rules.
Bounded repetition and over-used-word checks are planned as
flag-only. Mechanical word-choice swaps are planned with
auto-fix. All are list-driven, so they trend toward mdsmith.
Voice and subjective clarity stay with Vale.

## CI pipeline

Run the tools in parallel — they read the same files,
write nothing, and report independently:

```yaml
- name: vale
  run: vale docs/
- name: remark
  run: npx remark docs/ --frail
- name: mdsmith
  run: mdsmith check .
```

`mdsmith fix` rewrites files; Vale and remark stay
read-only by default, so there is no fight over a
single workspace.

## When to drop a tool

- Drop **remark** if you have no custom AST plugins.
  mdsmith covers the formatting checks remark presets
  ship.
- Drop **Vale** if your prose rules are all literal: banned
  words, banned openers, proper-noun casing, required terms.
  mdsmith's `forbidden-text`, `forbidden-paragraph-starts`,
  `proper-names`, `required-mentions`, and the `no-llm-tells`
  convention cover those today. Keep Vale for brand voice,
  passive-voice, and context-aware suggestions.
- Drop **mdsmith** if your only need is prose voice and
  you have no cross-file linking, generated sections,
  or release-gating on doc metrics. Vale is the simpler
  fit.

## See also

- [Linter comparison](../background/markdown-linters.md)
  — feature-by-feature breakdown across the Markdown
  linter landscape.
- [Cross-file integrity](../features/cross-file-integrity.md)
  — the mdsmith pillar that neither Vale nor remark has.
