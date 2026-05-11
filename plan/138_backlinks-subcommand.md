---
id: 138
title: "`mdsmith list backlinks` subcommand"
status: "✅"
model: sonnet
summary: >-
  Surface MDS027's link graph as a CLI subcommand.
  `mdsmith list backlinks <file>` lists every workspace
  file with a link to the target, optionally
  scoped by anchor. JSON output for agent / tooling
  consumers.
---
# `mdsmith list backlinks` subcommand

## Goal

Let a user (or an agent) ask "what links to
`docs/api.md`?" and get an answer in one
command. The link graph already exists inside
MDS027; this plan exposes it.

## Background

Plan **L-4** in the
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md)
records this as a small, high-leverage surface.
The trigger is the first agent or docs-team
question of the form "what depends on this
file?".

[MDS027](../internal/rules/MDS027-cross-file-reference-integrity/README.md)
already walks every link target during a check
pass. The backlink direction is the same graph
read in reverse.

## Non-Goals

- New parsing. The backlinks command reuses the
  link extractor MDS027 already runs.
- Wikilink coverage. Backlinks resolve the same
  link forms MDS027 sees today. Wikilinks fold
  in once L-1 ships (separate plan).
- Live updates. The command is a one-shot
  query. Watch / daemon mode is out of scope
  (tracked as P-2 in the research).
- Renaming. Following backlinks to rewrite them
  is the rename-refactor surface (L-3 / C-4).

## Design

### Invocation

```bash
mdsmith list backlinks docs/api.md
```

Output is one line per incoming link, sorted by
source path:

```text
docs/index.md:14: [API reference](api.md)
docs/getting-started.md:42: [api docs](./api.md)
plan/045_api-overhaul.md:8: [api](../docs/api.md)
```

Each line carries: source path, line number,
and the original link as it appears in the
source. The format is the same MDS027 reports
broken-link diagnostics in, so users do not
learn a second shape.

### Anchor scoping

```bash
mdsmith list backlinks docs/api.md#authentication
```

Returns only links whose anchor resolves to the
named heading. The slug uses the same rules
MDS027 applies for cross-file anchor checks.

### JSON output

```bash
mdsmith list backlinks --format json docs/api.md
```

```json
[
  {"source":"docs/index.md","line":14,
   "text":"API reference","target":"api.md"},
  {"source":"plan/045_api-overhaul.md","line":8,
   "text":"api","target":"../docs/api.md"}
]
```

The struct shape is one record per incoming
link with `source`, `line`, `text`, `target`.
Stable keys; agents can parse with a small
schema.

### Filtering

Two flags scope large workspaces:

- `--include GLOB` — only consider sources
  matching the glob. Repeatable.
- `--limit N` — cap output at N rows. Sorted
  output is stable, so `--limit` plus repeated
  invocations paginate naturally.

The defaults match `mdsmith check`: respect
`.mdsmith.yml` `ignore:`, follow the same
discovery walk, default-deny symlinks (plan 84).

### Performance

The link graph builds in the existing MDS027
pass. For a one-shot CLI run, the backlinks
command parses just the workspace once and
emits results. No persistence; no cache. If the
workspace grows past the point where this is
slow, the index work tracked as **P-1** is the
escape hatch — separate plan, separate
trigger.

## Tasks

1. Add `cmd/mdsmith/backlinks.go` with the new
   subcommand, wired through the same
   discovery and config loading the existing
   subcommands use.
2. Refactor the link-graph builder out of
   MDS027 into a shared package
   (`internal/linkgraph` is the natural home)
   so both the rule and the subcommand consume
   one implementation.
3. Implement the path-only and `path#anchor`
   query forms; resolve anchors via the same
   slug rules MDS027 uses.
4. Add `--format` (text / json), `--include
   GLOB`, `--limit N` flags.
5. Add a new doc page
   `docs/reference/cli/backlinks.md`. Link
   from the
   [CLI reference index](../docs/reference/cli.md).
6. Tests:

  - a file with three incoming links from
    distinct sources returns three rows,
  - anchor-scoped query filters correctly,
  - JSON output matches the documented shape,
  - `--include` and `--limit` combine
    correctly,
  - the graph builder behaves identically when
    invoked from MDS027 vs the subcommand
    (regression).

## Acceptance Criteria

- [x] `mdsmith list backlinks docs/api.md` returns
      every workspace link to that path, one
      per line, with source path and line.
- [x] `mdsmith list backlinks docs/api.md#auth`
      filters by resolved anchor.
- [x] `mdsmith list backlinks --format json` emits
      the documented JSON shape.
- [x] `--include GLOB` and `--limit N` scope
      the result.
- [x] MDS027 and the subcommand share one
      link-graph builder (no duplicated walk).
- [x] A new `docs/reference/cli/backlinks.md`
      page describes the subcommand with one
      worked example.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
