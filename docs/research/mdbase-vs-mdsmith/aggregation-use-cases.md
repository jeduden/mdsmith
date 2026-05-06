---
summary: >-
  Concrete use cases for query aggregations and the toughest open
  question for mdsmith — whether the index that makes them fast in
  mdbase has a stateless equivalent. Walks the workload shapes, the
  SQLite payoff, and the fzf / ripgrep-style alternative.
---
# Aggregation use cases and the index question

mdbase's strongest argument for a SQLite cache is
aggregation: grouping, counting, summing, joining
across typed records at vault scale. mdsmith does
not have aggregation today and does not have a
persistent index. This document walks the use
cases, looks at when the SQLite payoff is real
versus marginal, and explores whether mdsmith
could serve the same workloads stateless — the
way `fzf` and `ripgrep` do for their domains.

The point is to have a concrete picture of the
workload before committing to (or against) an
index. Triggers in
[learn-from-mdbase.md](learn-from-mdbase.md)
mention this doc; this is where they get
detailed.

## Aggregation workload shapes

Aggregation is not one thing. Five shapes recur
across teams that adopt mdbase or want to:

| Shape       | Example                                           | Cost driver              |
|-------------|---------------------------------------------------|--------------------------|
| count       | "How many open tasks per assignee?"               | scan all matching FM     |
| sum / avg   | "Total estimated hours per project."              | scan + arithmetic        |
| top-N       | "Five oldest unresolved bugs."                    | scan + sort              |
| join        | "List tasks whose assignee.role = 'reviewer'."    | scan + cross-file lookup |
| time bucket | "Plans completed per week over the last quarter." | scan + date math + group |

mdbase serves all five with a SQL query against
its index. mdsmith would have to scan files per
query today; with an index, it could serve them
the same way. With a stateless-fast approach (see
section 5), four out of five are reachable
without state.

## Operational mode matters more than tool choice

Each use case below names an **operational mode**
assumption before quoting numbers, because the
mode dominates the comparison far more than the
tool. There are two modes:

- **Cold-start mode.** A one-shot CLI invocation
  in a fresh process. CI runners (which spin up
  ephemeral containers per push), agent loops
  that exec subcommands, and ad-hoc terminal use
  all fall here. No persistent state survives
  between runs except what is on disk and in
  the OS file cache.
- **Long-lived-session mode.** A running process
  that handles many queries: an LSP server
  attached to an editor, a `mdbase watch`
  daemon, an interactive REPL. State (parsed
  AST, link graph, FM index) survives between
  queries.

Both tools can be evaluated under either mode.
The analyses below compare like-with-like:

| Mode               | mdbase                                           | mdsmith                                                                                  |
|--------------------|--------------------------------------------------|------------------------------------------------------------------------------------------|
| Cold start         | validate cache (stat + maybe hash) → query       | parse files in parallel → query                                                          |
| Long-lived session | watch mode keeps cache live → query is index hit | in-memory index built lazily / on-demand → query is index hit (P-1 / plan 131 territory) |

The persistent SQLite cache is **not** what
makes mdbase fast in long-lived mode; the live
in-memory state is. The cache exists to bridge
between sessions (or to skip parsing during
validation). Granting a running server to mdbase
means granting one to mdsmith too — and once
both have one, the parse work has already been
done in either tool. The numbers below reflect
this.

## Worked use cases

### A-1: Sprint dashboard

**Setting.** Engineering team tracks tasks as
Markdown files under `tasks/`. ~800 active tasks,
~5,000 historical. CI runs a "sprint dashboard"
job after every push that produces a status
summary as JSON, fed into a Slack notification.

**Mode assumption.** Cold start. CI runners are
ephemeral containers; cache state from previous
runs does not survive. Both tools start fresh
on every push.

**Query.** The query shape:

```yaml
types: [task]
where: status == "in-progress"
group_by: assignee
aggregate:
  - count
  - field: priority
    op: avg
order_by: count desc
```

**What it produces.** A two-column table:
assignee, in-progress task count, average
priority. ~10 rows.

**Cost driver.** 5,800 files. Per-file work is
parsing the FM (~100μs–1ms warm CPU).

**With mdbase, cold start.** No prior cache.
Either build the cache from scratch (~600ms of
parse work, comparable to a fresh re-read) and
then run the SQL, or skip the cache and walk
files. Total wall time roughly the same as
mdsmith stateless. **The "sub-millisecond" SQL
query only applies once the cache exists** — and
that requires either a previous run whose cache
survived (rare in CI) or a long-lived process
that built the index earlier.

**With mdsmith stateless.** Parallel re-read
through every `.md` file in `tasks/`, filter by
FM, count and average in-process. With ripgrep-
class IO and FM parsing in Go, ~600ms cold on
modern hardware. JSON output piped to the next
step.

**With either tool in long-lived mode.** Not
applicable to this use case — CI runs are
one-shot. If the team moved to a long-running
sprint-dashboard service that watched the
`tasks/` folder, both tools could serve queries
from a warm in-memory state in <1ms each. The
incremental cost of running a service for this
purpose is operationally larger than the
one-shot CI cost.

**Verdict.** Roughly equivalent for the CI use
case. ~600ms parse on either side is hidden in
the test pipeline. Switching to mdbase does not
buy speed here unless the team also adopts a
long-running indexer, at which point mdsmith's
in-memory equivalent (P-1 option 2 in
[learn-from-mdbase.md](learn-from-mdbase.md))
would close the gap symmetrically.

### A-2: Reviewer load report

**Setting.** Documentation team. ~3,000 RFCs.
Weekly report: "Show all RFCs whose author or
reviewer is X, grouped by status, with the
five-most-recent in each group."

**Mode assumption.** Cold start. Weekly cron
job in CI; no surviving cache.

**Query.** The query shape:

```yaml
types: [rfc]
where: 'author == "alice" || "alice" in reviewers'
group_by: status
aggregate:
  - count
  - field: created
    op: max
top_per_group: 5
order_by: created desc
```

**Cost driver.** 3,000 files. Per-file work the
same ~100μs–1ms.

**With mdbase, cold start.** Cache build (~300ms)

+ SQL query (~1ms) ≈ 300ms total. If the runner
preserves `.mdbase/` between weekly runs, only
files changed since last week need re-parse;
weekly delta on a stable corpus is small, so
~50ms validation + 1ms query ≈ 50ms. CI
preservation depends on caching strategy.

**With mdsmith stateless.** Parallel re-read,
filter by FM, sort and top-5 per group
in process. ~400ms cold.

**With either in long-lived mode.** Both serve
sub-ms after the first query in the session.
Not applicable to a weekly cron unless the team
runs a daemon for it.

**Verdict.** Cold-vs-cold both around 300–400ms.
mdbase wins meaningfully only when weekly
cache state is preserved between runs (CI cache
buckets), at which point it drops to ~50ms.
mdsmith would need a similar persistence
mechanism (P-1) to match. At 30,000 RFCs the
cold-start gap widens — both tools take ~3
seconds without preserved state — and either's
preserved-state version becomes the obvious
choice.

### A-3: Knowledge-graph backlinks at scale

**Setting.** Research lab vault. 12,000 notes
with dense `[[wikilinks]]`. Researcher opens a
paper note and wants the backlinks panel:
"What cites this paper?" In milliseconds.

**Mode assumption.** Long-lived editor session.
The researcher has the editor open all day;
queries run inside that session.

**Pre-conditions matter for this one.** Three
distinct points in the session:

- **Pre-1: cold session** — editor just opened,
  no warm state.
- **Pre-2: first backlink query** — researcher
  asks "what cites this?" for the first time.
- **Pre-3: subsequent backlink queries** —
  later in the same session.

**Query.** The query shape:

```yaml
backlinks_to: notes/chen-2024.md
group_by: tag
order_by: count desc
limit: 50
```

**Cost driver.** 12,000 files; the link graph is
the load-bearing structure. Each query needs an
edge lookup `(target → sources)` and then a group
by tag.

**With mdbase, watch mode running.** Across the three pre-conditions:

- Pre-1: at session start, mdbase has either a
  warm SQLite cache (from prior session) or
  rebuilds it (~2s for 12k files of full-body
  parse). With cache preserved, validation pass
  takes ~120ms.
- Pre-2: first query, indexed edge lookup,
  sub-millisecond.
- Pre-3: subsequent, also sub-ms.

**With mdsmith stateless CLI.** Across the same three pre-conditions:

- Pre-1: not applicable; CLI doesn't persist a
  session.
- Pre-2: re-read 12,000 files, ~2 seconds cold.
  Unacceptable as an interactive panel.
- Pre-3: same — every query is a cold start.

**With mdsmith in-memory link graph in the LSP
server (planned, P-1 option 2 + plan 131).**
Across the same pre-conditions:

- Pre-1: at LSP startup, walk the workspace and
  build the link graph in memory. ~2s on 12k
  files (parallel parse). Or build it in the
  background with priority queries (see
  "Background indexing" earlier in this doc),
  so the editor opens immediately and the
  index fills out.
- Pre-2: indexed edge lookup, sub-millisecond.
- Pre-3: sub-ms; the in-memory graph stays warm
  for the session.

**Verdict.** With the editor session
explicitly assumed, both tools serve queries in
sub-millisecond after the first cold build.
mdsmith CLI does not work for this use case at
this scale; mdsmith's planned LSP-resident link
graph does. The win for mdbase is the
cross-session warmth from the SQLite cache: a
researcher closing and reopening the editor
gets a faster Pre-1 with mdbase than with
mdsmith's in-memory-only model.

That cross-session warmth is the actual SQLite
payoff. Whether it justifies the cache
operational cost depends on how often the
editor restarts versus stays open all day.

### A-4: Time-bucketed velocity

**Setting.** Plan tracker. ~500 plans across
two years. Quarterly review: "Plans completed
per month, last 12 months."

**Mode assumption.** Cold start. Quarterly
report run by hand or in CI; no surviving
cache assumed.

**Query.** The query shape:

```yaml
types: [plan]
where: status == "✅" && completed_at >= today() - "365d"
group_by: time_bucket(completed_at, "1M")
aggregate: count
order_by: time_bucket asc
```

**Cost driver.** 500 files; light. The time-
bucket logic is the interesting part.

**With mdbase, cold start.** Cache build
(~50ms) + SQL with GROUP BY on bucket key
(~1ms) ≈ 50ms total.

**With mdsmith stateless.** Parallel parse, in-
process bucket, count. ~50ms even cold.

**With either in long-lived mode.** Sub-ms after
first query; not relevant for a quarterly job.

**Verdict.** Roughly equivalent at this scale
in cold-start mode. The corpus is small enough
that cache validation overhead and fresh parse
work converge. mdbase's GROUP BY syntax is
nicer to read; mdsmith would need Q-5
aggregations to match. The choice here is
expressiveness, not speed.

### A-5: Cross-type join

**Setting.** Mixed product wiki. RFC type lists
its `owner: alice`. Task type also has
`assignee: alice`. Query: "List Alice's open RFCs
with at least one in-progress related task."

**Mode assumption.** Either. The query is run
ad-hoc from the CLI in some teams, embedded in
a dashboard that polls in others. Numbers below
cover both modes.

**Query.** The query shape:

```yaml
types: [rfc]
where: |
  owner == "alice" &&
  status == "open" &&
  any(related_tasks, t -> t.asFile().status == "in-progress")
```

**Cost driver.** ~200 RFCs, ~5,000 tasks. Each
matching RFC triggers per-task lookups on
`related_tasks`, which is a list of paths.

**With mdbase, cold start.** Cache build
(~500ms for 5,200 files) + traversal-aware SQL
(2,000 index hits ≈ 10ms) ≈ 500ms total.

**With mdbase, long-lived (watch mode).** Cache
warm; ~10ms per query.

**With mdsmith stateless.** Re-read all RFCs
(filter, ~200ms), then read each related task
(200 RFCs × ~3 hops × ~1ms parse = 600ms). Total
~800ms cold.

**With mdsmith long-lived (LSP-resident link
graph + planned L-5 traversal).** Same ~10ms
per query as mdbase, after the cold index
build.

**Verdict.** Cold-vs-cold roughly comparable
(500–800ms). Long-lived mode lets either tool
serve queries in tens of milliseconds. The
SQLite cache pays for cross-session warmth, the
same as A-3. At larger scales (20k+ tasks, 4+
hops) the cold-start gap widens for both,
favoring whichever tool has a working long-
lived path.

### A-6: Real-time editor decoration

**Setting.** Researcher in VS Code wants
inline decoration: every wikilink shows the
target's title in light text after the link.
Updates as files change.

**Mode assumption.** Long-lived editor session
with LSP server attached. Decoration runs on
every keystroke that affects rendering, so this
is unequivocally a session-mode use case;
stateless CLI does not apply.

**Pre-conditions matter.** Four moments matter inside the session:

- **Pre-1: editor opens** — first decoration
  pass on the visible file.
- **Pre-2: typing inside the file** — link
  targets unchanged, decorations stable.
- **Pre-3: navigating to a different file** —
  new decorations.
- **Pre-4: external file change** — target
  file's title updated, decoration must
  refresh.

**Query.** Per visible link, fetch
`asFile().title`. Hundreds of times per file
open, called from the LSP server.

**Cost driver.** Latency. 50ms feels slow;
200ms is unacceptable.

**With mdbase, watch mode.** Across the four pre-conditions:

- Pre-1: ~120ms validation if cache exists,
  else ~2s rebuild. After that, decorations
  resolve sub-millisecond.
- Pre-2/3/4: sub-ms each, regardless of
  surrounding edits.

**With mdsmith stateless CLI.** Not applicable;
the CLI exits between queries. A per-decoration
shell-out would cost 5–10ms per resolution
just for process startup, plus disk read. Fails
all four pre-conditions.

**With mdsmith LSP-resident link graph (planned
P-1 option 2 + plan 131).** Across the same four
pre-conditions:

- Pre-1: ~2s cold rebuild, or instant if
  background indexing is in place and the
  editor opens before the index completes
  (decorations fill in as targets become
  resolvable).
- Pre-2: sub-ms (no link-target changes).
- Pre-3: sub-ms.
- Pre-4: relies on the LSP's
  `didChangeWatchedFiles` invalidating one
  file's cache entry; <10ms refresh.

**Verdict.** Both tools handle this in
long-lived mode. Stateless does not work for
either; the question is just whether the
long-lived process is mdbase-lsp or mdsmith's
LSP. Cross-session warmth (Pre-1 specifically)
is where mdbase's persistent cache wins;
restart frequency determines whether that
matters.

## What an index actually costs

Before deciding whether the index pays, the
estimate above ("seconds cold without cache,
sub-millisecond with cache") is too coarse. The
reality has three confounders that narrow the
band where a persistent cache wins.

### Sync-check overhead is not free

A persistent index has its own startup cost
before any query runs. At minimum it must
verify the cache is current. Two layers of check
exist (mdbase spec §13):

1. **mtime sweep.** `stat()` every file in the
   collection, compare against the cached
   mtime. A `stat()` is a few microseconds when
   the dirent is in the OS cache, more on a
   cold filesystem. 5,000 files × ~10μs ≈ 50ms.
   50,000 files ≈ 500ms.
2. **Hash on doubt.** When mtime is unreliable
   (network filesystems, build tools that
   `touch`, file-system clock skew across
   machines), the impl falls back to hashing
   file contents. SHA-256 of a 4KB file is
   ~30μs of CPU, plus the read. 5,000 files ≈
   150ms of hashing alone, more if the OS
   cache is cold.

Combined: a clean cache validation pass on a
50,000-file corpus is on the order of a second.
**That is in the same range as a fresh parallel
parse of the same corpus** (see the sketch
under "What a real benchmark plan would
measure" below). The cache wins only when the
saved work — parse time, link-graph build,
aggregation — clearly exceeds the validation
cost.

For a query like A-1 (sprint dashboard, 5,800
files, simple FM filter), the saved work is ~600ms
of parsing and a `WHERE` clause. The validation
cost is ~50ms. The cache wins, but by less than
the naive analysis suggests.

For a query like A-3 (backlinks panel, 12,000
files, link-graph lookup), the saved work is
~2 seconds of full-body parse plus link-edge
construction. Validation is ~120ms. Here the
cache pays clearly.

**This applies to mdbase too.** A one-shot
`mdbase query` invocation without watch mode
running has the same validation cost as a
hypothetical mdsmith with-cache run. The mdbase
SQLite cache wins decisively only in two regimes:

- **Long-lived sessions with watch mode.** The
  watcher (spec §15) keeps the cache live with
  the filesystem, amortizing validation across
  many queries. First-query latency stays high,
  but subsequent queries skip validation.
- **Repeated queries within one process.** The
  validation cost is paid once at startup; every
  query in the same session benefits.

For the `cd repo && mdbase query …` pattern that
matches a CI job or an agent loop, the SQLite
cache narrows the gap with stateless re-read
significantly. The dominant winning case for
SQLite over stateless is the always-on watch-mode
deployment, which mdbase explicitly designs for
(spec §15 mandates watch mode at conformance
level 6) but which fits a smaller share of
real-world workflows than the spec-first framing
implies.

### OS file cache is the implicit warm cache

Linux's page cache, macOS's UBC, Windows' system
cache — every modern OS keeps recently-read
files in RAM. The first `mdsmith check` reads
files from disk; the second reads them from
memory at memcpy speed.

For a typical project this matters more than
any application-level cache:

- A 50MB vault fits entirely in OS cache on any
  development machine. Once read, repeated reads
  are RAM-speed (≈10GB/s) for as long as the
  pages stay resident.
- The pages stay resident until evicted (LRU
  pressure, typically minutes-to-hours of idle)
  or the file is modified (the dirty page is
  flushed and re-read on next access).
- `stat()` results are cached the same way. A
  warm `stat()` is ~1μs.

The implication: **mdsmith's "stateless re-read"
is, in practice, a re-read from RAM**, not from
disk, for any corpus that fits in available
memory and any session that runs more than once.
A persistent application cache duplicates work
the OS already does for free. Where it wins is
*parsing* work — the conversion from raw bytes
to a structured index — which the OS does not
cache.

So the question shifts: at what corpus size, and
for what workload, does the parsing cost dominate
to the point where caching the parse result pays
for the cache's own validation overhead?

### Background indexing with priority queries

A third option sits between "rebuild on every
run" and "fully persisted index": **build the
index in memory at startup, in the background,
while queries run against the partial state and
get hoisted in priority.**

The pattern is well-trodden in IDEs.
IntelliJ and the TypeScript language server
both work this way: the editor opens
immediately, indexing starts in the background,
and any feature that needs not-yet-indexed data
either jumps the queue (re-prioritizing the
relevant subdirectory) or falls back to a
slower path until the index catches up.

For mdsmith the shape would be:

- **Cold start.** Spawn a background indexer
  goroutine that walks the workspace and parses
  files in priority order: `kind:` matches first,
  then files referenced by an open query, then
  the rest.
- **Query arrives.** The query thread checks
  the in-progress index. Files it needs but
  doesn't find get pushed to the front of the
  indexer's queue. Once parsed, the result is
  returned.
- **Tail.** The indexer keeps building until
  the workspace is fully indexed or the process
  exits.

This works **only when the process is
long-lived** — an LSP session, an
`mdsmith watch` daemon (P-2), or an interactive
REPL. For a one-shot CLI command (`mdsmith query
... && exit`) the process exits before the
background work finishes, so the value is
small. The pattern is therefore not a
replacement for stateless one-shot, but an
extension of the in-memory in-LSP option from
the P-1 alternatives table.

What it buys:

- **Apparent cold-start latency drops.** The
  user sees the first query result fast even on
  a 50,000-file workspace, because the indexer
  does not need to be complete to answer
  point queries.
- **No persistent state.** The whole index lives
  in process memory. Restart and rebuild; no
  staleness, no schema migration, no `.mdbase/`
  on disk.
- **Iteratively useful.** A workspace that's
  10% indexed can already answer 90% of "find
  this kind" queries when that kind is what was
  prioritized.

What it costs:

- Indexer state machine and priority queue
  (modest implementation work, well-understood
  pattern).
- Memory bounded by workspace size; mdsmith's
  existing 512MB GOMEMLIMIT covers most cases.
- More complex than "rebuild fresh per query",
  less complex than a persistent on-disk store.

### What this means for the trigger

The trigger for adding a persistent on-disk
cache (P-1 in
[learn-from-mdbase.md](learn-from-mdbase.md))
is therefore narrower than "cold-start cost
dominates". It needs to survive three filters
in sequence:

1. **OS-cache filter.** Repeated runs on a
   stable corpus are already fast through the
   page cache. A persistent cache helps only
   when the *parsed* index — not the raw bytes
   — is what's expensive to rebuild. Confirm by
   profiling the second invocation, not the
   first.
2. **Sync-check filter.** The savings from
   skipping parse work must clearly exceed the
   cost of validating the cache against the
   filesystem. This is the gap between "naive
   re-parse takes 1s" and "cache validation
   takes 0.5s".
3. **In-memory-with-priority filter.** A
   long-lived process can already get most of
   the latency benefit with background indexing
   in RAM, without the operational cost of an
   on-disk cache. Confirm the workload genuinely
   needs cross-process freshness or restart-time
   warmth.

Surviving all three is a higher bar than "cold
start is slow on a big repo". The wiggle room
is real and worth using before reaching for
persistent state.

### What a real benchmark plan would measure

For the choice to be informed rather than
guessed, the next step is concrete numbers on a
real corpus. A benchmark plan would compare
five configurations on synthetic 1k / 10k / 50k
file workspaces:

| Config                                | Cold start | 2nd run | Notes                             |
|---------------------------------------|------------|---------|-----------------------------------|
| stateless re-read, OS cache cold      | T_cold     | —       | upper bound; rare in practice     |
| stateless re-read, OS cache warm      | T_warm     | T_warm  | the realistic baseline            |
| in-memory index, lazy build, in-LSP   | T_warm     | <1ms    | for long-lived processes          |
| in-memory index with priority queries | <T_warm    | <1ms    | ditto, with apparent latency win  |
| persistent on-disk cache, mtime sync  | T_sync     | <1ms    | T_sync ≈ stat + index lookup      |
| persistent on-disk cache, hash sync   | T_hash     | <1ms    | T_hash > T_sync, paranoid setting |

The decision lands when `T_warm` for the
relevant workload exceeds an interactive
threshold (typically 100ms), the in-memory
options fall short for the access pattern, and
`T_sync` is comfortably below `T_warm`. Until
then, none of the persistent options earn their
operational baggage.

## What the mdbase query layer actually is

A few questions about the architecture come up
when reading the spec. The answers shape both
the cost picture and the security posture.

### Does the cache store the full body?

Spec §13 lists what the cache should track: file
metadata, parsed front matter, type assignments,
field values, **link graphs**, and **full-text
indexes**. It does not mandate "store the entire
body verbatim in a SQL row" — only that an FTS
index exists and that the cache is rebuildable
from files alone.

An SQLite-backed impl has two reasonable shapes,
both spec-conformant:

- **Content-in-FTS.** The full body lives inside
  an FTS5 virtual table. Cache size ≈ corpus
  size plus index. All body queries answered
  locally in SQLite.
- **External-content FTS.** FTS5 maintains the
  inverted index only; bodies stay on disk.
  Cache is much smaller. `file.body.contains`
  hits the index first; if the impl needs a
  context snippet, it re-reads the file.

Neither is mandated. The TS reference impl's
choice is an implementation detail not pinned
by the spec.

What this means for the workloads in this doc:

- **A-3 backlinks.** The link graph is a
  derived structure — `(source, target, anchor,
  kind)` rows — not body content. An impl can
  cache the link graph without storing body
  bytes at all. Storage mode is irrelevant.
- **A-6 editor decoration.** Needs the target's
  `title` (front matter), not body. FM cache
  alone is enough.
- **Q-3 body full-text search.** This is where
  body storage matters. FTS-only keeps the
  cache small; inline-content trades disk for
  fewer subsequent reads.

Building the cache reads every body once
regardless of mode (you cannot index without
reading the bytes). The first cold build cost
is identical between the two shapes; steady-
state size and re-read patterns differ.

For mdsmith's choice if it ever lands a cache
(P-1 in
[learn-from-mdbase.md](learn-from-mdbase.md)),
the same trade-off applies. Body-only workloads
that look like A-3 or A-6 don't need bodies in
the cache at all — caching the link graph and
FM is enough. Q-3-like workloads do need an FTS
index; whether bodies inline or external is a
size-vs-IO call.

### What the reference impls actually do

The spec is permissive about cache architecture.
There are two reference implementations of mdbase
today, and they make materially different
choices. Both affect the cost estimates above.

**TypeScript impl** ([`mdbase`][mdbase-ts]) —
used by `mdbase-cli`.

Reading [`src/cache/async-store.ts`][async-store]:

- **SQLite via `sql.js` (WebAssembly).** The
  cache uses [`sql.js`][sqljs], not a native
  driver. The entire database loads into RAM at
  process start, mutates in-memory, and writes
  back on `flush()`/`close()`. No incremental
  disk writes; no `mmap`.
- **Schema declares more than it populates.**
  Tables: `files` (path, mtime_ms, size,
  content_hash, frontmatter_json, body,
  types_json), plus `field_values` (denormalized
  FM with text/number/int columns and indexes),
  `links` (edge table), `tags`, and `meta`. The
  standard `upsertFile` writes only the `files`
  row. `field_values`, `links`, and `tags` are
  scaffolded but not populated by the basic
  cache writer.
- **Body inline, no FTS5.** Body is a TEXT
  column on `files`. Body queries scan linearly
  through in-memory strings.
- **Staleness via mtime_ms + size.**
  `content_hash` exists as a column but is not
  populated or checked in the code paths
  reviewed.
- **Link index built per query.** Each query
  that needs backlinks calls `buildLinkIndex`
  against the in-memory file cache, walks every
  file's typed link fields and body links, and
  builds a `Map<targetPath, Set<sourcePath>>`
  from scratch. The on-disk `links` table is
  not consulted.

**Rust impl** ([`mdbase-rs`][mdbase-rs]) — used
by `mdbase-lsp`.

Reading [`src/cache/schema.sql`][rs-schema] and
[`src/cache/indexer.rs`][rs-indexer]:

- **Native SQLite via `rusqlite` (bundled).**
  Real persistent SQLite file with incremental
  writes; no full memory load. This is the
  shape that makes "persistent cache" earn its
  name.
- **Schema actively populated.** Tables: `files`
  (path, mtime_ns, ctime_ns, size,
  frontmatter_json, body, effective_json,
  parse_error), `file_types` (path, type_name),
  `links` (source_path, target_path, location,
  field, raw_target), `unique_values` (type +
  field + value + path, for ID uniqueness),
  `meta`. The indexer writes to **all** of
  these on update.
- **Body inline, no FTS5.** Same as TS.
- **Staleness via mtime_ns** (nanosecond
  precision; TS uses ms). No content hash.
- **Effective FM cached.** `effective_json`
  stores the merged FM with defaults applied,
  so reads skip recomputing defaults.
- **Link table actively used.** Backlinks can be
  served from the indexed `links(target_path)`
  table — a B-tree probe rather than a per-
  query rebuild.

**Side-by-side at the storage layer.** The two impls in one table:

| Concern                 | TS impl (`mdbase`)           | Rust impl (`mdbase-rs`)              |
|-------------------------|------------------------------|--------------------------------------|
| SQLite binding          | `sql.js` (WebAssembly)       | `rusqlite` with bundled SQLite       |
| Persistence model       | In-memory; flush on close    | Incremental disk writes              |
| Body storage            | Inline TEXT column           | Inline TEXT column                   |
| FTS index               | None                         | None                                 |
| Staleness               | `mtime_ms` + `size`          | `mtime_ns` (+ separate ctime column) |
| Content hash            | Column exists, unused        | Not in schema                        |
| Effective FM cached     | No (recomputed)              | Yes (`effective_json`)               |
| `files` table populated | Yes                          | Yes                                  |
| `links` table populated | No (rebuilt per query)       | Yes                                  |
| `file_types` populated  | n/a (uses `types_json` blob) | Yes (denormalized rows)              |
| `unique_values`         | n/a                          | Yes (for ID uniqueness checks)       |
| Body queries            | Linear scan in RAM           | Linear scan via SQL                  |

**What this means for the cost estimates above.** Walking the worked use cases:

- **A-3 backlinks at 12,000 files.** The
  "indexed edge lookup, sub-millisecond" claim
  fits the **Rust impl** (B-tree on
  `links(target_path)`). The **TS impl** rebuilds
  the link index per query from in-memory FM +
  body — fast at RAM speed but tens of
  milliseconds, not sub-ms. The two impls have
  noticeably different latency on this workload.
- **A-6 editor decoration.** The Rust LSP can
  serve title lookups via direct table read on
  `file_types`/`files`. The TS impl pattern
  (rebuild per query) is fine for occasional
  decoration but not per-keystroke.
- **Q-3 body full-text search.** Linear scan
  either way — both impls store body inline
  with no FTS5. Acceptable for ad-hoc queries,
  not for interactive use at scale.
- **Cache size.** Both store body inline; cache
  size ≈ corpus size + FM/types overhead. TS at
  10,000 files of 5KB average = ~50MB; Rust
  similar.
- **Cold start to first query.** TS loads the
  whole cache file into RAM (~hundreds of ms
  at 50MB plus `sql.js` WASM init). Rust opens
  the SQLite file and runs queries against it
  via the OS page cache; cold start is much
  cheaper.

A different conforming impl — native SQLite
with FTS5, or a non-SQL store, or content-
addressed loose objects — would have other
trade-offs. The cost analysis in the rest of
this doc was written against the spec's
permissive picture; the actual impls sit at
two distinct points on the persistence /
indexing spectrum.

**For mdsmith's hypothetical cache** (P-1),
both impls are useful prior art:

- **TS-shape (in-memory + flush-on-close).** A
  persistent file that's fully loaded at
  startup. Operationally close to option 2
  (in-memory) in the P-1 alternatives table
  plus a save-on-exit step. Doesn't need
  SQLite's machinery; could be written as a
  Go `gob` blob or BoltDB.
- **Rust-shape (incremental persistent
  SQLite).** Real database with indexed access
  and small-disk-write update model. Closer to
  option 5 in the P-1 alternatives table. Pays
  for the cost of indexed joins, FTS5 if added,
  and concurrent reader/writer separation.

The choice between them depends on which
workloads matter and whether incremental writes
are needed during a session. mdsmith's existing
LSP-resident config cache already follows the
in-memory pattern; extending that to a link
graph and FM map (without persistence) is the
cheapest extension. Persistence — TS-shape or
Rust-shape — is a separate decision triggered
by cross-session warmth needs (see the trigger
analysis in
[learn-from-mdbase.md](learn-from-mdbase.md)).

[mdbase-ts]: https://github.com/callumalpass/mdbase
[mdbase-rs]: https://github.com/callumalpass/mdbase-rs
[async-store]:
  https://github.com/callumalpass/mdbase/blob/main/src/cache/async-store.ts
[query-engine]:
  https://github.com/callumalpass/mdbase/blob/main/src/operations/query-engine.ts
[rs-schema]:
  https://github.com/callumalpass/mdbase-rs/blob/main/src/cache/schema.sql
[rs-indexer]:
  https://github.com/callumalpass/mdbase-rs/blob/main/src/cache/indexer.rs
[sqljs]: https://github.com/sql-js/sql.js

### Does mdbase query body content beyond links?

Per spec §10, queries can reach the body via
`file.body`, which is "the raw markdown text
including syntax characters". Methods: substring
match (`.contains`), regex (`.matches`). That is
the full surface.

What this means in practice:

- **Yes:** "any file containing the word
  `OIDC`" is a one-line `where` clause.
- **No:** "any file whose first heading starts
  with `Migration`" — there's no AST access; the
  body is treated as a string.
- **No:** "any file with at least three H2
  headings" — same reason.
- **No:** "any file with a fenced code block
  tagged `cue`" — needs structural parsing.
- **Partial:** "any file that mentions a project
  name in its first paragraph" — only via regex
  on the prefix, no first-paragraph concept.

The body is a blob, not an AST. mdsmith's lint
engine has full AST access for every file (it
must, to lint structurally), but this isn't
exposed to `mdsmith query` today. Surfacing it
would be a Q-3 follow-on (see
[learn-from-mdbase.md](learn-from-mdbase.md)),
not a fundamental capability gap.

For users who want structural body queries
**today**, neither tool ships them. mdsmith
could add them more cheaply because the parse
already happens; mdbase would need to extend its
expression language and its index schema.

### Is the query language SQL?

No. mdbase's query language is the expression
DSL defined in spec §11: operators, string and
list methods, date functions, link traversal,
designed for compatibility with Obsidian Bases
syntax. Users do not write SQL.

Implementation-internal, an SQLite-backed impl
likely compiles the DSL down to SQL for the
cached path — translating

```text
status == "open" && priority >= 3
```

to roughly

```sql
SELECT path FROM files
WHERE json_extract(fm,'$.status') = 'open'
  AND json_extract(fm,'$.priority') >= 3
```

depending on schema. But the
compilation is implementation-defined and not
visible to users.

This means three things:

- The user-facing surface is restricted by
  design. There is no SQL drop-through, no
  `JOIN` syntax, no subqueries beyond the spec's
  `asFile()` traversal (depth-limited at 10).
- Different impls can use different storage:
  SQLite, in-memory map, Bolt-style KV, even a
  hypothetical PostgreSQL-backed impl. The DSL
  stays the same.
- Query compatibility across impls is the
  spec's contract; storage choice is private
  per impl.

mdsmith's `mdsmith query` uses CUE struct
literals as its query language, which is also
not SQL but is a richer constraint language at
the cost of less ergonomic value-comparison
syntax. Q-7 in
[learn-from-mdbase.md](learn-from-mdbase.md)
explores adding a Bases-compatible front-end.

### What is the security posture of the query layer?

mdbase's spec is largely silent on security;
each impl picks its own posture. The threat
surface for an SQLite-backed impl includes:

1. **Injection through the DSL.** If the impl
   compiles user expressions to SQL by string
   concatenation rather than parameter binding,
   crafted expressions could escape the
   intended query. Best practice is prepared
   statements with `?` placeholders for
   user-provided values; a careful impl never
   builds SQL from raw user input. The spec
   does not mandate this, so it depends on the
   impl's hygiene.
2. **Path traversal in link resolution.** Spec
   §8 mandates path sandboxing (`..` cannot
   escape the collection root). The
   `path_traversal` error code in appendix C
   confirms impls must reject escape attempts.
   Mostly a settled question.
3. **Resource exhaustion via traversal.** The
   spec sets a default `asFile()` depth limit
   of 10 and an expression nesting limit of 64
   levels. These bound deep walks but not
   wide ones — a query like
   `where: any(items, x -> any(x.deps, ...))`
   over a large list field can still be
   expensive. Impls add their own time/row
   budgets.
4. **YAML attacks on the parse step.** Anchor
   bombs ("billion laughs") and similar attacks
   are real on YAML-FM-heavy collections. The
   spec doesn't mandate a guard. mdsmith
   rejects YAML anchors and aliases by default
   (`internal/yamlutil`); an mdbase impl would
   need to do the same for parity.
5. **Cache file integrity.** The `.mdbase/`
   folder (typically gitignored) holds the
   SQLite file. If another process can write
   to it, false rows can be injected.
   Mitigation: the spec mandates the cache be
   rebuildable from files alone, so users can
   recover with `mdbase cache rebuild`. The
   attack window is "between rebuilds".
6. **Untrusted type definitions.** If
   `_types/*.md` are user-controlled (e.g. a
   PR adds a type), a hostile type file could
   declare expensive constraints, large
   `default_strict` impacts, or filesystem
   patterns that grow unboundedly. Impls
   typically treat type files as trusted; CI
   pipelines should scope where they come from.

mdsmith ran a 10-finding adversarial review
documented in
`docs/security/2026-04-05-adversarial-markdown.md`.
mdbase impls (TS, Rust LSP, CLI) have not
published equivalent reviews at the time of
writing. For projects that ingest untrusted
contributor Markdown, the practical guidance is
the same as for any tool that parses untrusted
input: validate inputs, bound resources,
parameterize queries, sandbox paths. The spec
helps with paths and depth; the rest is impl
quality.

### Side-by-side query-layer summary

| Concern                      | mdsmith                   | mdbase                             |
|------------------------------|---------------------------|------------------------------------|
| User query language          | CUE struct literal        | Bases-compatible DSL (spec §11)    |
| Body access in queries       | not yet (planned Q-3)     | yes (`file.body.contains/matches`) |
| Body structure access        | no (lint AST is internal) | no                                 |
| SQL surface to user          | none                      | none                               |
| Storage abstraction          | n/a (no persistent index) | DSL → impl-defined backend         |
| Path traversal sandbox       | yes (MDS027 + repo root)  | yes (spec §8 mandate)              |
| YAML anchor/alias guard      | yes (mdsmith hardening)   | impl-defined (spec silent)         |
| Resource limits in query     | rule timeouts (impl)      | depth ≤ 10, nesting ≤ 64           |
| Adversarial review published | yes (10 findings)         | not at the time of writing         |

Reading across: mdbase wins on body content
matching (because they shipped it). mdsmith wins
on hardened parsing today (because they did the
review). Both share the path-sandboxing baseline
the spec mandates.

## When operational mode + persistence pays

Reading across the six cases with the
mode-and-pre-condition framing, three signals
predict whether *some* form of cached or
indexed state earns its cost:

1. **Interactive latency.** Queries that run
   inside a UI (LSP decoration, backlinks panel,
   editor hover) need <100ms response. Stateless
   one-shot CLI does not get there at workspace
   scale; some form of long-lived state is
   required.
2. **Cross-session restart cost.** If a tool's
   long-lived process restarts often (editor
   close-and-reopen, agent loop respawn), warm
   state must be reconstructed each time. A
   persistent cache that survives restart pays
   here. A pure in-memory model rebuilds.
3. **Cross-file joins beyond one hop.** Single-
   file queries scale linearly with the corpus.
   Joins multiply the cost; any form of index
   (persistent or in-memory) turns each hop
   from O(n) to O(log n).

The cross-product, with explicit modes:

| Use case              | Mode (most natural) | Latency need | Joins?  | Cold-shot OK? | Long-lived helps? | Persistent cache adds? |
|-----------------------|---------------------|--------------|---------|---------------|-------------------|------------------------|
| A-1 sprint dashboard  | one-shot CI         | seconds      | no      | yes           | no (one-shot)     | only if CI preserves   |
| A-2 reviewer load     | one-shot weekly     | seconds      | one hop | yes           | not natural       | only if CI preserves   |
| A-3 backlinks panel   | long-lived editor   | <100ms       | one hop | no            | yes (decisive)    | cross-restart warmth   |
| A-4 velocity          | one-shot quarterly  | seconds      | no      | yes           | no (one-shot)     | minimal                |
| A-5 cross-type join   | either              | seconds–ms   | 2+ hops | yes (cold)    | yes (warm)        | cross-restart warmth   |
| A-6 editor decoration | long-lived LSP      | <50ms        | one hop | no            | yes (decisive)    | cross-restart warmth   |

Two patterns fall out:

- **Cold-shot use cases (A-1, A-2, A-4).**
  Roughly equivalent across both tools at
  cold-vs-cold. Persistent cache helps only if
  the runner preserves it (CI cache buckets).
  This is more about the deployment than the
  tool.
- **Long-lived use cases (A-3, A-5, A-6).**
  Both tools serve sub-millisecond after the
  first cold build *within* a session. The
  distinguishing axis is **cross-session
  warmth**: does state survive restart? mdbase's
  SQLite cache is the most direct way to make
  that warmth survive a process exit. mdsmith's
  current trajectory (in-memory in the LSP)
  rebuilds at session start; whether that's
  acceptable depends on how often sessions
  restart.

The interesting design question for mdsmith is
whether the two clear long-lived cases (A-3
backlinks panel, A-6 editor decoration) can be
served by an in-memory index inside the LSP
server, plus background indexing to mask cold
build, without ever shipping a
persistent on-disk cache.

## Stateless-fast: the `fzf` / `ripgrep` model

Both `fzf` and `ripgrep` are well known for being
fast without persistent state. They work because
of three properties:

1. **Aggressive parallelism.** Both tools fan out
   across all available cores. ripgrep walks the
   filesystem with concurrent producers and
   consumers; fzf processes input lines in
   parallel.
2. **Tight inner loops.** ripgrep's regex engine
   uses Aho-Corasick and Teddy SIMD matching;
   fzf's scoring is hand-tuned. Per-line cost
   is in the tens of nanoseconds.
3. **No fsync, no parsing of structure.**
   ripgrep matches bytes, not ASTs. fzf doesn't
   know what it's matching. The work that's
   skipped is the work that would need an index.

For an FM-aware tool like mdsmith, the analogy
holds for parts of the workload:

**What mdsmith could ripgrep-class.** Workloads where the stateless model holds:

- **Body full-text search (Q-3).** Already
  ripgrep's domain; mdsmith could shell out, or
  embed `regexp/syntax` and walk files
  in parallel.
- **FM filtering (Q-1, Q-2, A-1, A-4).** Reading
  YAML between `---` delimiters and matching a
  small struct against a CUE struct is cheap if
  parallelized. Estimate: 10,000 files in
  ~500ms cold on a modern laptop.
- **Aggregations on FM only (A-1, A-4).** Group
  and count after the parallel parse. The
  aggregation itself is microseconds; the parse
  dominates.

**What ripgrep-class hits limits on.** Workloads
where statelessness breaks down:

- **Backlinks at scale (A-3).** Building the link
  graph means parsing every link in every body,
  not just FM. ~10× more parse work. At 10,000
  files this is 5+ seconds cold — too slow for an
  interactive panel.
- **Multi-hop joins (A-5).** Each hop multiplies
  the work. Two hops over 10,000 files is
  50+ seconds without a cache.
- **Editor-LSP decoration (A-6).** Per-keystroke
  parsing is hopeless at any corpus size. The
  LSP server must keep state.

**The pattern.** Stateless-fast handles workloads
where each query touches each file exactly once
and the per-file work is small (<1ms). It breaks
down where the same data is touched repeatedly
(LSP decoration), where the access pattern is
graph-shaped (joins, backlinks at scale), or
where work per file is heavy (full-body parse).

## A pragmatic path for mdsmith

These design choices are open. The shape that
falls out of the workload analysis is:

1. **Make the stateless path as fast as possible
   first.** Parallel walk via the existing
   `internal/discovery` package, FM parse with
   `gopkg.in/yaml.v3`, in-process filtering and
   aggregation. Target: 10,000 files / 500ms
   cold. This lifts A-1, A-2, A-4 without any
   new state.
2. **Add an in-memory link graph in the LSP
   server.** Plan 131 already builds toward
   this. Extending the LSP's process-scoped
   state to cover backlinks (L-4) and decoration
   (A-6) costs nothing in the CLI workflow and
   handles the two cases that need <100ms
   response.
3. **Defer persistent on-disk index until a real
   trigger fires.** Real profiling on a real
   corpus showing CLI cold-start dominates. Or
   a feature like Q-5 aggregations whose joins
   make stateless infeasible at the user's
   scale. When it does fire, the choice between
   BoltDB-style and SQLite is a measurable one,
   not a guess.

The summary slogan, if one helps: **stateless by
default; in-memory in the LSP; persistent only
when the workload proves it.**

This is not a commitment. Triggers in
[learn-from-mdbase.md](learn-from-mdbase.md)
might fire in any order, and a real user case can
override any of these choices. The point of
walking the workloads here is to make sure the
choice is informed when it lands.

## Open questions

A real plan to act on any of this would have to
settle:

- **What is the actual cold-start cost today?**
  Benchmarks on a 1k / 10k / 50k file synthetic
  corpus would replace the estimates above.
- **Does parallel FM-only parse hit IO or CPU
  limits first?** Determines whether mmap or
  fanout-readers is the right shape.
- **Can the LSP in-memory graph subsystem be
  cleanly factored so that, if a CLI cache is
  ever added, both share one builder?** Reuse
  matters more than the storage choice.
- **What's the corpus-size threshold where each
  workload shape (A-1..A-6) tips into needing
  the index?** Should be measured, not guessed.
- **Is there a half-step: an "ephemeral"
  in-memory cache mode for one CLI invocation
  that runs many queries (e.g., a script
  block)?** Cheap to add, useful in agent loops.

## Sources

- [`fzf` README on architecture and SIMD scoring][fzf]
- [`ripgrep` "How fast is it" notes][rg]
- mdbase spec §10 (querying), §13 (caching), and
  appendix A.2 (task-management worked example)
- mdsmith codebase 2026-05: `internal/discovery`,
  `internal/lint/file.go`, `internal/query/query.go`

[fzf]: https://github.com/junegunn/fzf
[rg]: https://github.com/BurntSushi/ripgrep/blob/master/FAQ.md
