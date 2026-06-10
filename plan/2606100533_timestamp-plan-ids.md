---
id: 2606100533
title: 'Coordination-free plan ids from UTC creation timestamps'
status: "✅"
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
keys its dependency graph by filename and carried
duplicate-id workaround steps (removed by task 4). Matching `Plan <id>`
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
add one minute and check again. The same bump rule applies
when a renumbering-derived timestamp collides (see the
duplicate-pairs section). UTC, not local time, so ids from
containers in different timezones stay ordered.

The proto schema enforces the format. Legacy ids are
frozen as a closed range and new ids must be timestamps:

```cue
id: (int & >=1 & <=246) | (int & >=2601010000)
```

A `max+1` allocation like 244 now fails MDS020 instead
of silently extending the legacy sequence. The
id-equals-prefix pairing stays convention-only. The
PLAN.md catalog regenerates from real filenames, so a
mismatch surfaces on the next `mdsmith fix`.

### Why the id stays an integer

Every consumer types the id as "integer", none as "small
integer", so a 10-digit timestamp drops in unchanged:

- [proto.md](proto.md) — the `id:` and `depends-on:` CUE
  types accept any int.
- [PLAN.md](../PLAN.md) — `sort: numeric:id` orders the
  legacy ids first, then timestamp ids in creation order
  (with the 64-bit parse fix below for 32-bit targets).
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
parsed sort keys with `strconv.Atoi`, which returns the
platform int. Where int is 32 bits the parse fails, and
the catalog demotes the whole sort to string comparison
of the id field. That order interleaves legacy and
timestamp ids. The tinygo target that
[218_wasm-size-reduction.md](218_wasm-size-reduction.md)
defines (and [240_cuelite-drop-cue.md](240_cuelite-drop-cue.md)
delivers) is such a platform. The fix:
`strconv.ParseInt(value, 10, 64)`, following the
precedent in [`internal/config/size.go`](../internal/config/size.go).

### Legacy ids and the nine duplicate pairs

Existing plans keep their ids, with one exception. In
each duplicate pair, one twin moves to a timestamp id.
The unique-frontmatter rule can then pass on the whole
workspace. Derive the new id from the twin's first add
commit:

```bash
TZ=UTC git log --reverse --diff-filter=A \
  --date=format-local:'%y%m%d%H%M' --format=%cd \
  -- <file> | head -1
```

`format-local:` is required — plain `format:` ignores TZ
and prints the committer's recorded timezone. `--reverse`
plus `head -1` pins the first add event when a file has
several.

Derived timestamps collide: most twins entered the repo in
one batch import commit, so several files share one
creation minute. Resolve collisions with the standard bump
rule — process renumbered files in ascending path order
and add one minute until the id is free.

Choose the keeper of each legacy id by reference weight.
Keep the twin whose references are costlier to move —
live dependency chains, shipped diagnostic strings, and
config comments outweigh editable prose and comments.
Re-point the rest.

The executed split: 215 stays with the engine API. The
Obsidian WASM stack and three plan deps pin it. The
lines-only audit moves with its three Go comments and one
docs mention. 156 stays with kind-schema composition, the
`plan/156` comment in [.mdsmith.yml](../.mdsmith.yml).
Entry-unification moves, and the shipped
`see plan 156` diagnostic strings move with it to the
new id — left at 156 they would point users at the
wrong plan.

The live cuelite chain keeps 236 and 237. The completed
arch-fix twins move, updating the architecture-audit log
links. All nine moved twins are completed plans. No
`depends-on:` entry needed an edit — every ambiguous dep
already meant a keeper.

The race struck twice more while this plan was in review:
main minted duplicate pairs at 242 and 243, plus three
new singles. The same recipe absorbed them. Pairs 242 and
243 keep their live or referenced twins, and the
completed twins moved to timestamp ids. The frozen range
grew to 246 — each fresh max+1 plan widens it by one
until this lands, and the timestamp contract in proto.md
stops the sequence at the source.

Every reference to a moved id was re-pointed, not only
`depends-on:` entries. The sweep covered the whole repo
with non-digit boundaries: `plan/`, `docs/`, `.claude/`,
`.github/`, `internal/`, `cmd/`, `editors/`, `website/`.
Each hit was read to decide which twin it meant. The
cuelite chain means the cuelite files, not the arch-fix
twins.

One known orphan rides along:
[157_catalog-where-filter.md](157_catalog-where-filter.md)
declares `depends-on: [19]`, and no plan 19 exists in the
imported history. Delete that entry. A standing lint
check for dangling `depends-on:` targets stays out of
scope. pick-plan already treats unknown ids as unmet.

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

## Tasks

1. [x] A `numeric:id` catalog sort test in
   [`rule_test.go`](../internal/rules/catalog/rule_test.go)
   with an id above 2,147,483,647; `parseSortInt` moves to
   `strconv.ParseInt(value, 10, 64)`. (The test cannot go
   red on a 64-bit dev host; it pins the contract the
   32-bit target needs.)
2. [x] Renumber one twin of each duplicate pair (121, 153,
   156, 214, 215, 218, 219, 236, 237) per the design:
   derive the id, bump collisions in path order, rename
   the file, set its frontmatter `id:`, sweep the whole
   repo for references to the old shared id, and re-point
   each at the file its referrer means. Delete the
   orphaned `depends-on: [19]` entry in
   157_catalog-where-filter.md.
3. [x] Document allocation in the [proto.md](proto.md)
   comment block — the `date -u +%y%m%d%H%M` recipe, id
   equals filename prefix, bump one minute on collision —
   and tighten the proto `id:` type to
   `(int & >=1 & <=246) | (int & >=2601010000)`.
4. [x] Update the
   [pick-plan skill](../.claude/skills/pick-plan/SKILL.md):
   new plan ids are 10-digit timestamps; demote the
   duplicate-id gotchas to a history note; keep the
   dependency graph keyed by filename as defense in depth.
5. [x] Replace the max+1 instruction in
   [audit-checklist.md](../docs/development/architecture/audit-checklist.md)
   ("one above the highest existing prefix") with the
   timestamp recipe, pointing at proto.md as the canonical
   home. No separate docs page: proto.md owns allocation,
   the skill owns selection.
6. [x] Run `mdsmith fix PLAN.md` and confirm the table
   orders legacy ids ascending, then timestamp ids.

## Acceptance Criteria

- [x] Frontmatter ids are unique:
  `awk 'FNR==2,/^---$/ { if ($0 ~ /^id:/) print }' plan/*.md | sort | uniq -d`
  prints nothing. (Plain `grep '^id:'` is unsound — it
  also matches `id:` lines inside fenced code examples,
  e.g. in 92_file-kinds.md.)
- [x] Every `depends-on:` entry equals exactly one plan
  file's frontmatter id.
- [x] A catalog unit test sorts an id above 2,147,483,647
  under `sort: numeric:id`.
- [x] The [proto.md](proto.md) comment block names
  `date -u +%y%m%d%H%M`, and a plan with id 244 fails
  `mdsmith check` against the tightened `id:` type.
- [x] The pick-plan skill no longer instructs workarounds
  for live duplicate ids.
- [x] audit-checklist.md no longer says "one above the
  highest existing prefix".
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
