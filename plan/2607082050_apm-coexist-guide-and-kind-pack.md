---
id: 2607082050
title: "Coexist-with-APM guide and primitive kind pack"
status: "🔲"
model: sonnet
summary: >-
  Ship docs/guides/coexist-with-apm.md with the canonical `ignore:`
  and `overrides:` set that keeps `mdsmith fix` off APM's
  hash-pinned deployed files, plus a copy-paste kind pack that lints
  the `.apm/` primitive sources. Opportunities C-2, A-1, A-2, L-1.
depends-on: [2607082048]
---
# Coexist-with-APM guide and kind pack

## Goal

Give a repo that runs both mdsmith and APM one guide
that says exactly which files mdsmith must leave
alone and which kinds to apply to the `.apm/` source
it should lint. Today both invariants are expressible
in config, but no page assembles them, so every team
rediscovers the boundary.

## Background

[APM](https://github.com/microsoft/apm) deploys
third-party Markdown into committed paths
(`.github/prompts/`, `.claude/rules/`,
`.agents/skills/`, and more) and pins a SHA-256 per
file in `apm.lock.yaml`. A repo-wide `mdsmith fix`
rewrites those bytes and trips `apm audit --ci`. The
same files were authored to their package's
conventions, not the consumer's, so `mdsmith check`
fails on files the team cannot edit. Both problems
are laid out in the
[APM workflows analysis](../docs/research/apm-mdsmith/workflows.md)
and catalogued as C-2, L-1, A-1, and A-2 in the
[opportunity catalogue](../docs/research/apm-mdsmith/opportunities.md).

The `.apm/` source tree is the opposite case: it is
the author's own Markdown, with contractual front
matter APM's docs state but never enforce. mdsmith
kinds are the checker — the exact pattern the repo
already runs for its own `skill` and `plan` kinds.

This guide follows the pattern of the
[Prettier guide](../docs/guides/coexist-with-prettier.md)
and the
[Vale + remark guide](../docs/guides/coexist-with-vale-and-remark.md).

## Non-Goals

- Foreign managed-region support inside the compiled
  files. That is
  [plan 2607082049](2607082049_foreign-managed-regions.md);
  this guide references it and, until it lands, tells
  users to `ignore:` the compiled root files.
- Schema grammar changes. Closed frontmatter and
  filename agreement are
  [plan 2607082051](2607082051_apm-schema-extensions.md);
  the kind pack here uses only today's grammar.
- Running APM. The guide documents coexistence, not
  APM setup.

## Tasks

1. Write `docs/guides/coexist-with-apm.md` with an
   ownership table: which tool owns each file class
   (APM-deployed, APM-compiled, `.apm/` source,
   hand-authored).
2. Ship the canonical `ignore:` list naming APM's
   deploy directories, plus `overrides:` disabling
   fix-capable rules on the compiled root files
   (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`,
   `.github/copilot-instructions.md`) when
   APM-managed.
3. Ship the kind pack: `apm-skill`, `apm-prompt`,
   `apm-instruction`, and `apm-agent` kinds with
   `path-pattern` and inline frontmatter schemas
   matching the
   [primitive contracts](../docs/research/apm-mdsmith/apm-model.md),
   plus per-kind `max-file-length` and `token-budget`
   (500 lines / 5000 tokens for `SKILL.md`, 300 lines
   for agents). The `apm-prompt` kind opts its content
   rules into the
   [`${input:name}` token](2607082048_apm-input-placeholder-token.md).
4. Add the guide to the features/guides catalog and
   cross-link it from the research README.
5. Verify the shipped config: create a scratch `.apm/`
   fixture and confirm `mdsmith check` passes on a
   conformant tree and flags a missing `description`.
6. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [ ] `docs/guides/coexist-with-apm.md` exists and
      passes `mdsmith check` under the guides kind.
- [ ] The guide's `ignore:` block, pasted into a
      consumer `.mdsmith.yml`, stops `mdsmith fix`
      from rewriting a file under `.github/prompts/`.
- [ ] The kind pack flags a `.apm/skills/x/SKILL.md`
      missing `name` or `description`.
- [ ] The kind pack flags an `.apm/agents/x.agent.md`
      body over 300 lines.
- [ ] The guide is reachable from the guides catalog
      and the research README.
- [ ] `mdsmith check .` — 0 failures.
