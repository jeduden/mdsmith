---
summary: >-
  Deep-dive comparison of mdsmith and mdbase — two file-first, plain-
  Markdown-on-disk approaches to the same substrate, with different
  surfaces today and overlapping evolutionary candidates.
---
# mdsmith vs mdbase

This research note compares
[mdsmith](https://github.com/jeduden/mdsmith) — a Go
Markdown linter — with [mdbase](https://mdbase.dev) — a
specification for typed, queryable Markdown collections,
with a TypeScript reference implementation, a Node CLI,
and a Rust LSP. Both treat plain `.md` files on disk as
the source of truth and refuse to introduce a separate
artifact format. The substrate they read is the same
(YAML front matter, Markdown body, link graph). What
differs today is which surfaces each tool exposes
over that substrate. Neither tool's exposure is a
permanent split — see
[learn-from-mdbase.md](learn-from-mdbase.md) for the
candidate directions and the cost-vs-benefit triggers
for each.

## Documents in this folder

- [README.md](README.md) — this overview
- [features.md](features.md) — feature-by-feature
  comparison: distribution, config, type system,
  validation, queries, links, generated content,
  prose linting, conventions, cache, CLI, LSP,
  output formats
- [workflows.md](workflows.md) — way-of-working
  comparison: daily authoring, repo bootstrap, schema
  evolution, file rename, CI, editor session,
  Obsidian-vault use, LLM/agent use
- [use-cases.md](use-cases.md) — seven concrete
  worked scenarios (open-source repo, Obsidian
  vault, RFC tracker, task tracker, agent-
  maintained runbooks, knowledge graph, mixed
  wiki+plan tracker) showing which tool fits each
  use case best **today**
- [interop.md](interop.md) — running both tools on
  the same files: coexistence, the dual-schema
  problem, recommended layouts, future bridge
- [learn-from-mdbase.md](learn-from-mdbase.md) —
  systematic gap enumeration: every mdbase
  capability mdsmith doesn't yet expose, with a
  per-gap mini-plan covering goal, sketch, surface,
  effort, and the **trigger** under which shipping
  it becomes worth the cost. mdsmith's feature set
  is open; this doc maps candidates.
- [aggregation-use-cases.md](aggregation-use-cases.md)
  — six worked aggregation workloads (sprint
  dashboard, reviewer-load report, knowledge-graph
  backlinks, time-bucketed velocity, cross-type
  join, real-time editor decoration), the cases
  where a SQLite-class index pays vs. where it
  doesn't, an analysis of the costs an index
  actually carries (sync-check, OS file cache,
  background indexing with priority queries), and
  a serious look at stateless-fast (`fzf` /
  `ripgrep`-style) approaches as an alternative.
- [query-and-types.md](query-and-types.md) —
  in-depth comparison of the query languages
  (CUE struct literal vs Bases-compatible DSL)
  and the type systems. Walks what each tool
  actually types (FM? body? filenames?), how
  schemas compose, the expressiveness matrix
  between the two languages, and the answer to
  whether mdbase's type system is limited to
  front matter (almost — with two named
  exceptions).

## TL;DR

Two tools, one substrate, different surfaces today.

**Same substrate.** Both projects read plain `.md`
files with YAML front matter and Markdown body.
Both can in principle parse front matter, walk a
link graph, evaluate expressions over the result,
and produce diagnostics. There is no fundamental
capability split between the two projects —
the underlying data model is shared.

**Different surfaces today.**
[mdsmith](https://github.com/jeduden/mdsmith)
ships a Go binary that focuses on linting and
fixing: 54 rules covering prose readability,
structural conventions, generated sections
(catalog, TOC, include, build), front-matter
schema validation via [MDS020][mds020]
(`proto.md` schemas with CUE in the front matter),
and a CUE-struct-literal `query` subcommand. No
persistent state, stateless re-read per run.
[mdbase](https://mdbase.dev) is a specification
plus reference impls (TS, Rust LSP, Node CLI)
that focuses on typed records: a 12-type FM
field taxonomy, CRUD operations, an expression
DSL compatible with Obsidian Bases for queries,
optional SQLite-backed caching, watch mode, and
rename with link rewriting.

Shared by both projects:

- Files on disk are authoritative
- No proprietary format
- Offline operation
- YAML front matter as the structured layer
- Both can grow toward each other; neither has a
  fixed feature set

**Where the surfaces don't yet meet.** The
[learn-from-mdbase.md](learn-from-mdbase.md)
catalogue describes ~30 capabilities present in
mdbase that mdsmith does not yet expose, with a
**trigger** for each — the concrete condition
under which shipping that capability becomes
worth the cost. None of the entries are
out-of-bounds; they all sit somewhere on a
cost-vs-benefit curve. Symmetrically, mdbase
does not expose mdsmith's surface either — prose
linting, regenerable sections, a fix engine —
and could in principle grow toward those.

## When to use which today

The table below describes which tool best serves
each workflow **with the surfaces shipped as of
2026-05**. None of these are permanent
assignments; the candidate-evolution column
points at the trigger if it exists.

| Workflow                                      | Best fit today             | Candidate for the other side |
|-----------------------------------------------|----------------------------|------------------------------|
| Lint Markdown structure and style             | mdsmith                    | —                            |
| Enforce paragraph readability or token budget | mdsmith                    | —                            |
| Auto-generate catalogs, TOCs, includes        | mdsmith                    | —                            |
| Reformat tables, fix trailing whitespace      | mdsmith                    | —                            |
| Validate front-matter shape                   | either                     | shared today                 |
| Query files by front-matter field             | either                     | shared today                 |
| Rename a file and update all backlinks        | mdbase                     | mdsmith L-3 / C-4            |
| Compute backlinks for a knowledge graph       | mdbase                     | mdsmith L-4                  |
| Watch a folder for changes (event stream)     | mdbase                     | mdsmith P-2                  |
| Index 10k+ files for fast queries             | mdbase                     | mdsmith P-1                  |
| Run in CI as a single static binary           | mdsmith                    | —                            |
| Edit a vault in Obsidian with typed schemas   | mdbase                     | mdsmith S-1..S-3             |
| Block PRs on broken cross-file links          | mdsmith ([MDS027][mds027]) | —                            |
| Block PRs on missing front-matter fields      | either                     | shared today                 |
| Wikilink validation                           | mdbase                     | mdsmith L-1                  |
| Aggregations (group / count / avg)            | mdbase                     | mdsmith Q-5                  |

The "candidate" column references mini-plan IDs
from [learn-from-mdbase.md](learn-from-mdbase.md).
Each carries its own trigger condition.

## Status snapshot (2026-05)

| Property         | mdsmith                           | mdbase                                  |
|------------------|-----------------------------------|-----------------------------------------|
| Spec version     | n/a (single impl)                 | 0.2.1                                   |
| Reference impl   | mdsmith (Go)                      | mdbase (TS), mdbase-rs (Rust LSP)       |
| Distribution     | one Go binary                     | npm package + CLI + LSP                 |
| Maturity         | stable rules, MDS029 experimental | early release; conformance levels 1–6   |
| License          | MIT                               | MIT                                     |
| Language         | Go 1.25+ (per `go.mod`)           | TypeScript / Rust                       |
| Runtime deps     | none                              | Node 22+ (TS impl); Rust LSP standalone |
| Network          | none                              | none                                    |
| Persistent state | none                              | optional SQLite cache                   |
| Plugin system    | no (rules baked in)               | no (spec is fixed; impls vary)          |

## Same substrate, different surfaces

Both tools read the same on-disk substrate.
What each currently exposes from that substrate
is a snapshot, not a charter.

```text
                ┌────────────────────────────────────────┐
                │ Shared on-disk substrate (.md files)   │
                │   YAML front matter + Markdown body    │
                │   + cross-file link graph              │
                └────────────────────────────────────────┘
                                  │
              ┌───────────────────┴────────────────────┐
              │                                        │
              ▼                                        ▼
    mdsmith surfaces today                  mdbase surfaces today
    ─────────────────────                   ─────────────────────
    - lint diagnostics + fix                - typed CRUD operations
    - 54 rules (prose, structure)           - field-typed schemas
    - generated sections                    - rename + ref rewrite
      (catalog, TOC, include, build)        - Bases query DSL
    - CUE FM schema (MDS020)                - SQLite cache
    - LSP: diagnostics, code actions        - watch mode events
    - merge driver                          - LSP: completion, hover,
    - CUE query                               go-to-definition

   Each surface has a `proto.md`-style       Each surface has a typed
   schema as the contract; both             record as the contract;
   parse the same FM bytes.                 both parse the same FM bytes.

                ┌────────────────────────────────────────┐
                │ Candidate evolutions either way        │
                │   see learn-from-mdbase.md +           │
                │   aggregation-use-cases.md             │
                └────────────────────────────────────────┘
```

The "candidate evolutions either way" row is the
point. mdsmith does not have wikilinks, rename
refactor, or backlinks today; each is a candidate
with a trigger. mdbase does not have prose
linting, generated sections, or a fix engine
today; the same applies in reverse if anyone
takes that direction. The substrate supports
both halves; the surfaces are choices, made
under cost-vs-benefit.

What teams running both feel today is friction
in the **schema layer specifically** — both
tools want to validate the same FM, and neither
reads the other's schema language.
[interop.md](interop.md) walks the workarounds.

## Reading order

For a tool comparison, start with
[features.md](features.md) for the mechanical
breakdown, then [workflows.md](workflows.md) for
daily-work feel, then [use-cases.md](use-cases.md)
for seven concrete scenarios that show where each
tool fits today, and finally [interop.md](interop.md)
for combining both.

For a design exercise, read
[learn-from-mdbase.md](learn-from-mdbase.md):
every gap with a mini-plan and a trigger. Then
dive into
[aggregation-use-cases.md](aggregation-use-cases.md)
for the toughest open question — when an index
pays — including a serious look at stateless-fast
(`fzf` / `ripgrep`-style) approaches as an
alternative to a persistent cache.

## Sources

- mdbase specification:
  <https://github.com/callumalpass/mdbase-spec>
  (sections 0–15, appendices A–D, version 0.2.1)
- mdbase project site:
  <https://mdbase.dev>
- mdbase TypeScript reference impl:
  <https://github.com/callumalpass/mdbase>
- mdbase CLI: <https://github.com/callumalpass/mdbase-cli>
- mdbase LSP (Rust):
  <https://github.com/callumalpass/mdbase-lsp>
- mdsmith codebase as of 2026-05 (54 rules,
  Go 1.25+ per `go.mod`, MIT). The research
  pass landed in the PR that introduced this
  folder; commit history under
  `docs/research/mdbase-vs-mdsmith/` is the
  authoritative trail.

[mds020]: ../../../internal/rules/MDS020-required-structure/README.md
[mds027]: ../../../internal/rules/MDS027-cross-file-reference-integrity/README.md
