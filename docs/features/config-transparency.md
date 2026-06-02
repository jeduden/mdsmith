---
title: "Config you can explain"
summary: >-
  Config layers deep-merge rule by rule: defaults, convention,
  kinds, then overrides. `--explain` and `mdsmith kinds resolve`
  show which layer set each effective value, per leaf.
icon: git-compare
link: "/reference/cli/kinds/"
weight: 17
---
# Config you can explain

Four config layers stack on every rule. When a rule fires, the
question is which layer set the value that triggered it — mdsmith
answers that per leaf.

Config resolves in order: defaults, then convention, then kinds,
then per-glob overrides. The merge is rule by rule. Maps merge
key by key, so a later layer that touches one setting does not
erase its siblings. Scalars replace; lists replace unless a rule
opts a setting into append.

`check --explain` attaches provenance to every diagnostic.
`mdsmith kinds resolve <file>` prints the effective config with
per-leaf provenance, and `kinds why <file> <rule>` shows the full
merge chain including no-op layers.

See the [kinds reference](../reference/cli/kinds.md) and the
[CLI reference](../reference/cli.md) for merge semantics.
