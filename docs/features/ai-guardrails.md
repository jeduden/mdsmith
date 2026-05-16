---
title: "Guardrails for AI-generated docs"
summary: >-
  Cap file, section, and token-budget size; enforce reading grade and
  sentence count; flag verbatim copy-paste across files.
icon: bot
link: "/docs/guides/metrics-tradeoffs/"
rules: ["MDS022", "MDS028", "MDS037"]
weight: 4
---
# Guardrails for AI-generated docs

Generated documentation drifts toward bloat: long files, padded
sections, and the same boilerplate pasted everywhere. mdsmith caps
each axis with a rule.

Size rules cap
[file](../../internal/rules/MDS022-max-file-length/README.md),
[section](../../internal/rules/MDS036-max-section-length/README.md),
and
[token-budget](../../internal/rules/MDS028-token-budget/README.md)
length. Prose rules enforce a
[reading grade](../../internal/rules/MDS023-paragraph-readability/README.md)
and a
[sentence count](../../internal/rules/MDS024-paragraph-structure/README.md).

[`MDS037`](../../internal/rules/MDS037-duplicated-content/README.md)
flags verbatim copy-paste across files, so a generator cannot pad
the corpus by repeating itself.

See the [metrics trade-offs guide](../guides/metrics-tradeoffs.md)
for threshold guidance.
