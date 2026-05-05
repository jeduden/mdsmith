---
summary: Deep-dive comparison of mdsmith and mdbase — two file-first, plain-Markdown-on-disk approaches with non-overlapping responsibilities.
status: 🔳
---
# mdsmith vs mdbase

This research note compares
[mdsmith](https://github.com/jeduden/mdsmith) — a Go
Markdown linter — with [mdbase](https://mdbase.dev) — a
specification for typed, queryable Markdown collections,
with a TypeScript reference implementation, a Node CLI,
and a Rust LSP. Both treat plain `.md` files on disk as
the source of truth and refuse to introduce a separate
artifact format. They differ on which layer of the
Markdown stack they own.

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
  wiki+plan tracker) that exercise where each
  tool fits
- [interop.md](interop.md) — running both tools on
  the same files: coexistence, the dual-schema
  problem, recommended layouts, future bridge
- [learn-from-mdbase.md](learn-from-mdbase.md) —
  systematic gap enumeration: every mdbase
  capability mdsmith doesn't have, with a per-gap
  mini-plan covering goal, sketch, surface,
  effort, and the **trigger** under which
  shipping it becomes worth the cost. mdsmith's
  feature set is open; this doc maps candidates
  rather than committing to any.
- [aggregation-use-cases.md](aggregation-use-cases.md)
  — six worked aggregation workloads (sprint
  dashboard, reviewer-load report, knowledge-graph
  backlinks, time-bucketed velocity, cross-type
  join, real-time editor decoration), the cases
  where a SQLite-class index pays vs. where it
  doesn't, and a serious look at stateless-fast
  (`fzf` / `ripgrep`-style) approaches as an
  alternative to a persistent cache. The deepest
  open design question for mdsmith.

## TL;DR

The two tools are complementary, not competitive.

**mdbase** is a specification for typed Markdown
collections. It defines how a folder of `.md` files
becomes a queryable, schema-validated database.
Implementations provide CRUD, schema typing, link
resolution, rename refactoring, query expressions
(Bases-compatible), and an SQLite-backed cache. The
spec is at version 0.2.1 (early release) with
reference implementations in TypeScript, a Node CLI,
and a Rust LSP.

**mdsmith** is a single Go binary that lints, fixes,
and regenerates Markdown. It owns prose readability,
structural rules, generated sections (catalog, TOC,
include, build), and a CUE-based front-matter schema.
54 rules ship in the binary. Stable today, no
network, no daemon, no cache.

Both projects:

- Treat the filesystem as authoritative
- Refuse a proprietary file format
- Ship offline, single-binary or single-package
- Target the same Obsidian-and-Git audience
- Read YAML front matter as the structured layer

The overlap region is **front-matter typing and
querying**. mdsmith's [file kinds][file-kinds] and
[required-structure][mds020] rule cover the same
ground as mdbase types, but with different syntax
(CUE vs an mdbase-specific schema DSL) and different
ownership (lint config vs `_types/` Markdown files).
The non-overlap region is **everything else**: prose
rules, fixers, and regenerable sections live only in
mdsmith; rename refactoring, the query language, and
the SQLite cache live only in mdbase.

## When to use which

| Need                                          | Reach for                  |
|-----------------------------------------------|----------------------------|
| Lint Markdown structure and style             | mdsmith                    |
| Enforce paragraph readability or token budget | mdsmith                    |
| Auto-generate catalogs, TOCs, includes        | mdsmith                    |
| Reformat tables, fix trailing whitespace      | mdsmith                    |
| Validate front-matter shape                   | either                     |
| Query files by front-matter field             | either                     |
| Rename a file and update all backlinks        | mdbase                     |
| Compute backlinks for a knowledge graph       | mdbase                     |
| Watch a folder for changes (event stream)     | mdbase                     |
| Index 10k+ files for fast queries             | mdbase                     |
| Run in CI as a single static binary           | mdsmith                    |
| Edit a vault in Obsidian with typed schemas   | mdbase                     |
| Block PRs on broken cross-file links          | mdsmith ([MDS027][mds027]) |
| Block PRs on missing front-matter fields      | either                     |

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

## Same problem, different layer

A picture helps. Both tools sit on the same files but
own different layers of behavior.

```text
            ┌──────────────────────────────────────┐
            │ Markdown source files (.md)          │
            │   YAML front matter + body content   │
            └──────────────────────────────────────┘
                          │
        ┌─────────────────┼──────────────────┐
        │                 │                  │
        ▼                 ▼                  ▼
   mdsmith owns:     overlap region:    mdbase owns:
   - prose rules     - FM typing        - rename refactor
   - structure       - FM querying      - backlink graph
   - readability     - kind/type        - watch events
   - catalog, TOC      assignment       - SQLite index
   - include, build  - schema           - link traversal
   - merge driver      validation       - CRUD operations
   - LSP diagnostics                    - Bases queries
   - 54 lint rules                      - migrations
```

The "overlap region" is where teams running both will
feel friction: today there is no way to share the
schema. mdsmith reads CUE; mdbase reads its own DSL.
[interop.md](interop.md) covers the workarounds.

## Reading order

For a tool comparison, start with
[features.md](features.md) for the mechanical
breakdown, then [workflows.md](workflows.md) for
daily-work feel, then [use-cases.md](use-cases.md)
for seven concrete scenarios that show where each
tool fits, and finally [interop.md](interop.md)
for combining both.

For a design exercise, read
[learn-from-mdbase.md](learn-from-mdbase.md):
every gap mdsmith has against mdbase, sketched as
a mini-plan with a concrete trigger for when
shipping it would pay. Then dive into
[aggregation-use-cases.md](aggregation-use-cases.md)
for the toughest open question — when, if ever,
mdsmith should grow an index — including a
serious look at stateless-fast (`fzf` /
`ripgrep`-style) approaches as an alternative to
a SQLite cache.

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
  Go 1.25+ per `go.mod`, MIT). The original
  research pass landed in the PR that introduced
  this folder; commit history under
  `docs/research/mdbase-vs-mdsmith/` is the
  authoritative trail.

[file-kinds]: ../../guides/file-kinds.md
[mds020]: ../../../internal/rules/MDS020-required-structure/README.md
[mds027]: ../../../internal/rules/MDS027-cross-file-reference-integrity/README.md
