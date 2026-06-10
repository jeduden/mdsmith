---
id: 2606100533
title: 'Coordination-free plan ids from UTC creation timestamps'
status: "🔲"
model: sonnet
summary: >-
  Allocate plan ids as the minute-precision UTC creation
  timestamp (date -u +%y%m%d%H%M) instead of max+1, so two
  branches cannot pick the same id. Renumber the nine
  legacy duplicate pairs and make the catalog numeric sort
  64-bit.
---
# Coordination-free plan ids from UTC creation timestamps

## Problem

A new plan file takes `max(existing ids) + 1`. The max is
read at branch time, but a competing branch only becomes
visible at merge time. Both merges land, because the two
filenames differ in slug and git sees no conflict. Nine id
pairs collided this way: 121, 153, 156, 214, 215, 218,
219, 236, 237.

A shared id breaks every place where the id is the key.
`depends-on: [215]` resolves to two files. The
[pick-plan skill](../.claude/skills/pick-plan/SKILL.md)
keys its dependency graph by filename and carries
duplicate-id workaround steps. Matching `Plan <id>`
against branch names and PR titles returns two candidates.

The PLAN.md catalog table is not the problem: the merge
driver already regenerates it on conflict. The race is in
id allocation itself, and any scheme that derives the next
id from the currently visible set re-creates it.

## Goal

Make plan-id allocation coordination-free. The id becomes
the minute-precision UTC creation timestamp. Two sessions
then collide only when both create a plan in the same UTC
minute. The unique-frontmatter rule
([2606100534](2606100534_unique-frontmatter-rule.md))
makes that residue fail loudly.

## Design

### Allocation

At creation, run `date -u +%y%m%d%H%M` and use the output
as both the frontmatter `id:` and the filename prefix.
This plan's own id, 2606100533, encodes 2026-06-10 05:33
UTC. If a `plan/<id>_*.md` already exists in the checkout,
add one minute and check again. UTC, not local time, so
ids from containers in different timezones stay ordered.

### Why the id stays an integer

Every consumer types the id as "integer", none as "small
integer", so a 10-digit timestamp drops in unchanged:

- [proto.md](proto.md) — `id: 'int & >=1'` and
  `depends-on: '[...int]'` both accept it.
- [PLAN.md](../PLAN.md) — `sort: numeric:id` orders the
  legacy ids (largest: 243) first, then timestamp ids in
  creation order.
- The plan kind `path-pattern` in
  [.mdsmith.yml](../.mdsmith.yml),
  `plan/{proto.md,[0-9][0-9]*_*.md}`, matches with no
  config edit.
- The [pick-plan skill](../.claude/skills/pick-plan/SKILL.md)
  matches ids as `\d+` with non-digit boundaries, so
  branch names `plan-<id>-<slug>` and PR titles
  `Plan <id>: <title>` keep their shape.

### 64-bit sort key

A timestamp id exceeds 2,147,483,647, the 32-bit int
maximum. `parseSortInt` in
[`internal/rules/catalog/rule.go`](../internal/rules/catalog/rule.go)
parses sort keys with `strconv.Atoi`. That returns the
platform int. On a 32-bit target the parse fails, and the
catalog falls back to lexical order. Lexical order mixes
legacy and timestamp ids. The tinygo build that
[240_cuelite-drop-cue.md](240_cuelite-drop-cue.md) aims
for is such a target. Parse with
`strconv.ParseInt(value, 10, 64)` instead.

### Legacy ids and the nine duplicate pairs

Existing plans keep their ids, with one exception: in each
duplicate pair, one twin moves to the timestamp of its own
first commit:

```bash
TZ=UTC git log --diff-filter=A \
  --date=format:'%y%m%d%H%M' --format=%cd -- <file>
```

Keep the legacy id on the twin that references outside
`plan/` name. The `plan/156` comment in
[.mdsmith.yml](../.mdsmith.yml) means kind-schema
composition, so that twin keeps 156. With no outside
reference, keep the earlier-created twin. Each
`depends-on:` entry naming a formerly shared id moves to
the id of the file its referrer means. Read the referrer
to decide: the cuelite chain means the cuelite files, not
the arch-fix twins.

### Rejected alternatives

- A next-id counter file: turns every pair of concurrent
  plans into a guaranteed textual merge conflict — the
  failure mode this plan removes.
- GitHub issue numbers as ids: allocation would need the
  network and GitHub at plan-creation time; the repo
  otherwise works fully offline.
- A random integer suffix: the same residual collision
  odds without the chronological sort or the
  self-describing creation date.
- The slug as the identity: retypes `depends-on:` to
  strings and rewrites the id schema, the PLAN.md sort,
  and every id regex in pick-plan, for no smaller race
  window.

## Tasks

1. [ ] Red/green: a `numeric:id` catalog sort test in
   [`rule_test.go`](../internal/rules/catalog/rule_test.go)
   with an id above 2,147,483,647; make it pass by moving
   `parseSortInt` to `strconv.ParseInt(value, 10, 64)`.
2. [ ] Renumber one twin of each duplicate pair (121, 153,
   156, 214, 215, 218, 219, 236, 237) per the design:
   rename the file, set its frontmatter `id:`, grep
   `plan/`, `docs/`, and `.claude/` for references to the
   old shared id, and re-point each `depends-on:` entry at
   the file its referrer means.
3. [ ] Document allocation in the [proto.md](proto.md)
   comment block: the `date -u +%y%m%d%H%M` recipe, id
   equals filename prefix, bump one minute on collision.
4. [ ] Update the
   [pick-plan skill](../.claude/skills/pick-plan/SKILL.md):
   new plan ids are 10-digit timestamps; demote the
   duplicate-id gotchas to a history note; keep the
   dependency graph keyed by filename as defense in depth.
5. [ ] Add `docs/development/plan-ids.md` with a
   `summary:` frontmatter stating the scheme and this
   rationale; run `mdsmith fix .` so the CLAUDE.md and
   PLAN.md catalogs include it.
6. [ ] Run `mdsmith fix PLAN.md` and confirm the table
   orders legacy ids ascending, then timestamp ids.

## Acceptance Criteria

- [ ] `grep -h '^id:' plan/*.md | sort | uniq -d` prints
  nothing.
- [ ] Every `depends-on:` entry equals exactly one plan
  file's frontmatter id.
- [ ] A catalog unit test sorts an id above 2,147,483,647
  under `sort: numeric:id`.
- [ ] The [proto.md](proto.md) comment block names
  `date -u +%y%m%d%H%M` as the id source.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
