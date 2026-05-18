---
title: "Guardrails for AI-generated docs"
summary: >-
  Cap file, section, and token-budget size; enforce reading grade and
  sentence count; flag verbatim copy-paste across files.
icon: bot
link: "/guides/metrics-tradeoffs/"
rules: ["MDS022", "MDS028", "MDS037"]
weight: 4
---
# Guardrails for AI-generated docs

Generated documentation drifts toward bloat: long files, padded
sections, and the same boilerplate pasted everywhere. mdsmith caps
each axis with a rule.

Size rules cap file (`MDS022`), section (`MDS036`), and
token-budget (`MDS028`) length. Prose rules enforce a reading
grade (`MDS023`) and a sentence count (`MDS024`). `MDS037` flags
verbatim copy-paste across files, so a generator cannot pad the
corpus by repeating itself. The
[rule directory](/rules/)
has the full reference for each.

See the [metrics trade-offs guide](../guides/metrics-tradeoffs.md)
for threshold guidance.
