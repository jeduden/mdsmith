---
summary: >-
  How mdsmith can support APM (Microsoft's Agent Package Manager)
  workflows: APM's Markdown-heavy package model, a two-axis sweep of
  its workflows and subcommands, and a catalogue of opportunities
  with per-item mini-plans.
---
# mdsmith and APM

This research note analyzes how mdsmith — a Go Markdown linter —
can support, improve, or augment workflows built around
[APM](https://github.com/microsoft/apm), Microsoft's Agent Package
Manager. APM installs and compiles AI-agent context (skills,
prompts, instructions, agents) across ten harnesses. Its payload
is Markdown with contractual YAML front matter; its outputs are
committed files sitting next to — sometimes inside — the files
mdsmith already lints and generates.

The analysis is systematic on two axes: every user workflow the
APM docs describe (axis 1) and every `apm` subcommand (axis 2).
Both axes were swept against a complete map of mdsmith's shipped
features, so each finding is labeled with whether mdsmith covers
it today, almost covers it, or needs a new feature.

## Documents in this folder

- [README.md](README.md) — this overview
- [apm-model.md](apm-model.md) — APM's package model as a linter
  sees it: manifest, lockfile, the seven primitive types and their
  front-matter contracts, compile targets, and the validation APM
  ships today
- [workflows.md](workflows.md) — axis 1: the 29 APM user
  workflows, their Markdown surfaces, and the five clusters where
  mdsmith changes the outcome
- [subcommands.md](subcommands.md) — axis 2: all 32 `apm`
  subcommands and the Markdown each reads or writes
- [opportunities.md](opportunities.md) — the merged catalogue:
  every opportunity with status (exists / partial / new), effort,
  trigger, and a mini-plan

## The one-paragraph conclusion

APM validates dependency integrity (hashes, lockfile, hidden
Unicode) and stops where content quality starts. mdsmith owns
content quality (schemas, style, size, links, generated sections)
and knows nothing about APM's file shapes. The pairing needs three
things: an APM preset (kinds and schemas for `SKILL.md`,
`*.prompt.md`, `*.instructions.md`, `*.agent.md`, plus a
`${input:…}` placeholder token), a coexistence posture for
consumer repos (ignore/override boundaries so `mdsmith fix` never
rewrites hash-pinned deployed files, and awareness of APM's
`<!-- apm:start -->` managed sections), and a documented CI
composition (`apm audit --ci` beside `mdsmith check .`). Most of
the machinery exists; the connective config, one vocabulary token,
and the docs do not.

## Getting started

`mdsmith init --apm` scaffolds both the kind pack and the
coexistence ignore posture in one step. See
[Coexist with APM](../../guides/coexist-with-apm.md) for the full
ownership table, quick-start instructions, and CI ordering.
