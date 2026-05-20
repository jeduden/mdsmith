---
title: Coexist with Vale and remark
summary: >-
  Vale owns brand voice and prose style; remark owns
  Markdown AST transformations; mdsmith owns formatting,
  cross-file integrity, and generated sections. They sit
  side by side in CI without overlap.
---
# Coexist with Vale and remark

Vale, remark, and mdsmith solve different problems.
Most docs teams run two of them; a few run all three.
This page draws the boundary so each tool gets the
scope it is best at.

## Who owns what

| Concern                                   | Owner   |
|-------------------------------------------|---------|
| Brand voice and prose style               | Vale    |
| Inclusive-language and proper-noun checks | Vale    |
| Custom prose rules tied to a style guide  | Vale    |
| Markdown AST transformations              | remark  |
| Custom Markdown plugins                   | remark  |
| Whitespace, heading style, code fences    | mdsmith |
| Bare URLs, link reference integrity       | mdsmith |
| Generated sections (catalog, toc)         | mdsmith |
| Cross-file link and anchor integrity      | mdsmith |
| File kinds and per-directory schemas      | mdsmith |
| Readability budgets (ARI, sentence count) | mdsmith |

Readability appears in both columns: Vale has
proselint-style readability rules, mdsmith has
`paragraph-readability` (ARI). Pick one as the source of
truth for that signal so writers do not see the same
warning twice.

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
- Drop **Vale** if you do not maintain a brand-voice
  style. mdsmith's readability budget and forbidden-text
  rule cover the common cases.
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
