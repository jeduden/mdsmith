---
summary: >-
  Concrete worked scenarios that exercise the differences between
  mdsmith and mdbase, showing where each tool fits naturally and where
  the gaps surface.
---
# Use cases and worked examples

This document is a set of concrete scenarios. Each
takes a real-shaped task and walks through what the
team would do with mdsmith, with mdbase, and with
both. The point is to make the abstract differences
in [features.md](features.md) feel mechanical: when
do they actually bite?

## Use-case index

| #   | Scenario                                   | Best fit       |
|-----|--------------------------------------------|----------------|
| U-1 | Open-source repo: docs/ tree               | mdsmith        |
| U-2 | Personal Obsidian vault (PKB)              | mdbase         |
| U-3 | RFC / spec tracker with status fields      | mdsmith (lean) |
| U-4 | Issue / task tracker as Markdown           | mdbase (lean)  |
| U-5 | AI-agent-maintained runbooks               | mdsmith        |
| U-6 | Knowledge graph with backlinks and renames | mdbase         |
| U-7 | Mixed: product wiki + typed plan tracker   | both, layered  |

Each scenario uses the same structure:

- **Setting** — two or three sentences of context
- **Sample file** — the front matter and body shape
- **What the team needs** — a list of capabilities
- **mdsmith approach** — concrete commands and config
- **mdbase approach** — concrete commands and config
- **Verdict** — which tool fits and why

## U-1: Open-source repo with `docs/` tree

**Setting.** A Go library with a `docs/` folder. 120
Markdown files: README, getting-started, reference
docs, contributor guide, plus per-feature explainers.
Twenty contributors over four years. PRs from
strangers come in regularly.

**Sample file.** Front matter and body shape:

```markdown
---
title: Configuring transport timeouts
summary: How to tune the http.Client timeout knobs.
---
# Configuring transport timeouts

Three timeout knobs decide how the client behaves on
slow networks: connect, response-header, and
end-to-end. Set them with…
```

**What the team needs.** Concrete capabilities they want:

- Block PRs on broken cross-file links
- Enforce heading conventions
- Auto-generate a doc-index table from front matter
- Catch verbose paragraphs and overlong files
- Run in CI as a single binary
- Zero install friction for new contributors

**mdsmith approach.** `go install` on the binary;
`mdsmith check .` in CI. The repo's `.mdsmith.yml`
sets line length, paragraph readability (ARI),
heading rules, and a `<?catalog?>` directive at the
top of `docs/index.md` that rebuilds on
`mdsmith fix`.

**mdbase approach.** None of the team's needs map
onto mdbase. The vault has no typed records, no
backlinks worth indexing, and no rename refactor
load. Adding `mdbase.yaml` would not help.

**Verdict.** mdsmith is the natural fit. The repo
is text-with-front-matter, and the value is in
prose / structure linting and generated indexes.
mdbase would be inert.

## U-2: Personal Obsidian vault

**Setting.** A solo knowledge-worker keeping ~3,000
Markdown notes in an Obsidian vault committed to
Git. Daily journal, project notes, meeting notes,
recipes, book reviews, all linked with `[[wikilinks]]`.

**Sample file.** Front matter and body shape:

```markdown
---
type: daily
date: 2026-05-04
mood: focused
energy: 6
---
# 2026-05-04

Worked on [[mdsmith]]. Closed a thread on
[[plan-131]]. Read [[Designing Data-Intensive
Applications]] chapter 3.

## tomorrow
- review [[plan-122]]
```

**What the team needs.** Concrete capabilities they want:

- Validate front matter: `date` is ISO, `mood` is
  one of a known set, `energy` is 1–10
- Show backlinks: every page that mentions
  `[[plan-131]]`
- Rename `plan-131` to `plan-131-lsp-symbols` and
  rewrite every wikilink across the vault
- Query "all daily notes from 2026-Q1 with
  energy >= 7"
- Fast operation at 3,000 files

**mdsmith approach.** mdsmith treats `[[wikilinks]]`
as plain text. MDS027 cannot validate them. There
is no rename refactor and no backlink index. Front
matter validation works (CUE in proto.md), but
that's a small slice of the need.

**mdbase approach.** Native fit. `_types/daily.md`
declares the schema. Wikilinks resolve through the
L4 link parser. `mdbase rename plan-131 plan-131-lsp-symbols`
rewrites every incoming link. SQLite cache makes
queries instant. The Rust LSP gives the editor
hover, completion, and backlinks.

**Verdict.** mdbase. mdsmith has nothing to add to
a wikilink-heavy vault other than line-length and
paragraph-readability rules — useful but secondary.

## U-3: RFC / spec tracker

**Setting.** A platform team writing 60 long-form
RFCs over two years. Each RFC follows a strict
template: problem, alternatives, decision,
implementation. Front matter carries id, title,
authors, status, and a `supersedes:` chain.

**Sample file.** Front matter and body shape:

```markdown
---
id: RFC-027
title: Object storage abstraction
authors: [alice, bob]
status: accepted
created: 2026-02-14
supersedes: [RFC-019]
---
# RFC-027: Object storage abstraction

## Problem

## Alternatives

## Decision

## Implementation
```

**What the team needs.** Concrete capabilities they want:

- Enforce the four-section heading template
- Validate front matter (status is one of a fixed
  set, id matches `RFC-NNN`, dates are ISO)
- Generate an index table of all RFCs sorted by id
- Block PRs on broken `supersedes:` references
- Catch readability regressions in long sections

**mdsmith approach.** `kinds: [rfc]` with
`required-structure.schema: rfcs/proto.md`. The
`proto.md` carries the four-heading template and
CUE for the front matter. `<?catalog?>` builds the
index table. MDS023 / MDS024 catch readability.
MDS027 validates link integrity. The team gets
everything in one tool.

**mdbase approach.** `_types/rfc.md` declares the
typed schema. Status enum, id pattern, ISO dates
all map cleanly. Queries find blocked RFCs.
Backlinks find supersession chains. But: no
heading-template enforcement, no readability
checks, no index-table generation in-place.

**Verdict.** mdsmith (lean). The four-section
template is the structural backbone of the artifact;
mdsmith owns that. mdbase types add typed-data
nicety, but the team can get most of the same
guarantees from CUE in the mdsmith proto.md.
Pairing both works (see [interop.md](interop.md)),
but mdsmith alone covers more of the need.

## U-4: Issue / task tracker as Markdown

**Setting.** A small team tracking work as
Markdown files in `tasks/`. Each task is a record
with status, priority, assignee, due date, and a
description body. ~500 tasks active at any time.
Heavy CRUD: tasks created / updated / closed
several times per day.

**Sample file.** Front matter and body shape:

```markdown
---
id: TASK-0231
title: Migrate auth to OIDC
status: in-progress
priority: 3
assignee: alice
due: 2026-06-01
---
# TASK-0231: Migrate auth to OIDC

The current SAML flow has two open issues. We will
swap to OIDC over the next sprint.
```

**What the team needs.** Concrete capabilities they want:

- Create a new task with `id` and `created` filled
  automatically
- Update `status` from the CLI without opening the
  file
- Query "open tasks, priority >= 3, due in 7 days"
- Rename `TASK-0231` to `TASK-0231-oidc-migration`
  and update every reference
- Compute "days overdue" without storing it
- Watch a folder and notify on changes

**mdsmith approach.** mdsmith does not author or
edit front matter. Task creation and update happen
in the editor or via shell. `mdsmith query` filters
by FM but cannot sort or compute days-overdue.
Renames are manual; MDS027 catches the breakage.
There is no watch mode beyond the LSP per-session.

**mdbase approach.** Native CRUD. `mdbase create task`
fills in `id` (ULID), `created` (`now_on_write`),
defaults. `mdbase update TASK-0231 --status done`
edits the file. Bases query covers sort, filter,
and the computed `days_overdue` field.
`mdbase rename` plus `mdbase watch` round out the
workflow.

**Verdict.** mdbase. This is the canonical use case
mdbase was designed for. mdsmith can lint task
files for prose quality and structural rules, but
the day-to-day data plane is mdbase's territory.

## U-5: AI-agent-maintained runbooks

**Setting.** A platform team's runbook set lives
under `runbooks/`. ~80 files, each describing a
service incident response. An AI agent (Claude
Code) is part of the maintenance loop: it ingests
postmortems, updates runbooks, generates new ones,
and reorganizes the set on demand.

**Sample file.** Front matter and body shape:

```markdown
---
service: auth-gateway
severity: high
last-incident: 2026-04-22
on-call-rotation: platform
---
# Auth gateway: 5xx spike

## Symptoms

## First steps

## Escalation

## Verification
```

**What the team needs.** Concrete capabilities they want:

- Agent verifies its edits with deterministic exit
  codes
- Agent regenerates the index table at
  `runbooks/index.md` after creating files
- Editor LSP exposes outline, go-to-definition,
  and "what depends on this runbook?" so the agent
  can navigate
- Block PRs that break the heading template or
  cross-file links
- No network calls during lint or fix

**mdsmith approach.** `mdsmith check .` for
verification, `mdsmith fix .` for regenerating the
catalog and TOC, JSON output for parsing. After
plan 131 lands, the LSP gives the agent
documentSymbol, definition, and call hierarchy. No
network anywhere.

**mdbase approach.** mdbase-lsp gives the agent
typed-vault navigation: completion of field names,
hover with field constraints. `mdbase create runbook`
scaffolds new files. But mdbase does not lint
prose, fix tables, or regenerate the index.

**Verdict.** mdsmith is the agent's primary tool.
mdbase-lsp adds value once typing is in place but
is not the foundation. After plan 131 lands the
agent gains the document-graph view it needs to
plan multi-file edits.

## U-6: Knowledge graph with renames

**Setting.** A research lab's shared vault. ~12,000
Markdown files with dense `[[wikilink]]` cross-
references. Files get renamed often as concepts get
sharper. Backlinks are how researchers discover
related work.

**Sample file.** Front matter and body shape:

```markdown
---
tags: [transformer, attention, scaling-laws]
authors: [chen-2024]
---
# Notes on Chen et al. (2024)

The paper extends [[scaling-laws]] to mixture-of-
experts and confirms the [[chinchilla-law]]
prediction with one caveat. Compare with
[[Kaplan-2020]].
```

**What the team needs.** Concrete capabilities they want:

- Backlinks panel: "what cites this paper?"
- Rename `chinchilla-law` to `chinchilla-2022`
  across 12,000 files in seconds
- Search by tag, by author, by year
- Trust the rename: zero broken links after
- Index that scales to 12,000 files

**mdsmith approach.** Scales poorly. No backlinks,
no wikilink awareness, no rename refactor.
Per-run re-read of all files makes interactive use
slow. mdsmith does not solve this problem.

**mdbase approach.** Designed for it. SQLite cache
keeps backlinks instantaneous. Wikilink resolution
is L4. Rename rewrites all incoming links in one
operation. Watch mode keeps the index live.

**Verdict.** mdbase, decisively.

## U-7: Mixed product wiki + typed plan tracker

**Setting.** A 30-engineer team. The repo holds
a product wiki (`docs/`, prose-heavy, generated
indexes) AND a plan tracker (`plan/`, typed
records with status / owner / acceptance
criteria). The two surfaces share a single vault
because plans link to docs and vice versa.

**Sample plan file.** Plan record shape:

```markdown
---
id: 145
title: Multi-tenant key rotation
status: in-progress
owner: alice
acceptance:
  - rotation runs without downtime
  - documented in docs/security/key-rotation.md
---
# Plan 145: Multi-tenant key rotation
```

**Sample doc file.** Doc shape:

```markdown
---
title: Key rotation
summary: How keys are rotated across tenants.
---
# Key rotation

See [plan 145](../../plan/145_key-rotation.md) for
the rollout schedule.
```

**What the team needs.** Concrete capabilities they want:

- Lint prose in `docs/`
- Validate plan front matter (status enum, id pattern)
- Generate the plan index table
- Backlinks: "what plans link to this doc?"
- Cross-file integrity: "does the plan's
  acceptance reference a real doc?"

**mdsmith approach.** Covers all the docs concerns
plus plan-front-matter validation via a `plan`
kind plus a `proto.md` schema. MDS019 generates
the index. MDS027 validates cross-file links. No
backlinks.

**mdbase approach.** Plan tracker is a perfect fit
for typed schemas; docs are out of scope. `_types/plan.md`
typifies the plan; backlinks panel surfaces "what
plans cite this doc". Bases queries find blocked
plans.

**Verdict.** Both, layered. mdsmith owns the docs
prose / structure / catalog work and validates plan
front matter via proto.md. mdbase types the plan
records, surfaces backlinks for the
plans-cite-docs graph, and feeds queries the team
runs interactively. See
[interop.md](interop.md) section 4 for a folder
layout that runs both cleanly.

## Pattern across the seven cases

| #   | Prose / structure load | Typed-record load | Rename / backlink load | Best fit       |
|-----|------------------------|-------------------|------------------------|----------------|
| U-1 | high                   | low               | low                    | mdsmith        |
| U-2 | low                    | medium            | high                   | mdbase         |
| U-3 | high                   | medium            | low                    | mdsmith (lean) |
| U-4 | low                    | high              | medium                 | mdbase (lean)  |
| U-5 | high                   | low               | low                    | mdsmith        |
| U-6 | low                    | medium            | very high              | mdbase         |
| U-7 | high                   | high              | medium                 | both           |

Three signals predict the fit:

1. **Prose / structure load** — does the artifact
   have prose that needs linting and generated
   sections that need regenerating? If yes, mdsmith.
2. **Typed-record load** — are files records with
   constrained shapes whose types matter at write
   time? If yes, mdbase.
3. **Rename / backlink load** — does the team
   rename often, and do incoming links matter? If
   yes, mdbase.

A team scores each axis. Two-or-three highs in
the same column point at one tool; mixed scores
point at running both.
