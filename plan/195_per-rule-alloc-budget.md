---
id: 195
title: Enforce the ≤ 10 allocs/op per-rule budget across every registered rule
status: "✅"
model: opus
depends-on: [193]
summary: >-
  CLAUDE.md documents a "≤ 10 allocs per call on representative
  input" ceiling for every rule's Check, but only MDS024 has a
  benchmark that fails CI when the rule crosses it. A
  parametric gate that runs each registered rule against one
  shared representative fixture catches the budget violation
  for every rule at once. The first run flags 13 rules that
  exceed the ceiling; this plan adds the gate, fixes each
  rule to land at ≤ 10 allocs, and closes the
  default-vs-parity gap on the engine corpus benchmark.
---
# Enforce the ≤ 10 allocs/op per-rule budget across every registered rule

## Goal

Every rule registered with `rule.Register` allocates ≤ 10
times per `Check` call on the shared representative
fixture. A single `go test` gate enforces this. The
fixture exercises a typical Markdown document: heading,
prose, fenced code, link, list, table, and reference
link. The gate measures what one Check pays on real
input — not a worst-case microbenchmark.

The ceiling lives in [docs/development/index.md][budget].
This plan turns that prose into an enforceable gate.

[budget]: ../docs/development/index.md

## Background

[Plan 193](193_mds024-allocation-budget.md) shipped
MDS024's per-rule alloc gate (cold lint.File minus parse
baseline). The shape is reusable: every rule that
implements `rule.Rule.Check(*lint.File) []lint.Diagnostic`
can be measured the same way. A shared fixture lets one
gate cover all rules, with the per-rule sub-test naming
any regression.

The first run of the new gate found these failures on
the representative fixture:

| Rule   | Name                               | allocs/op | Default? |
|--------|------------------------------------|----------:|---------:|
| MDS029 | conciseness-scoring                | 443       | opt-in   |
| MDS035 | toc-directive                      | 198       | opt-in   |
| MDS025 | table-format                       | 62        | default  |
| MDS026 | table-readability                  | 36        | default  |
| MDS027 | cross-file-reference-integrity     | 30        | default  |
| MDS054 | no-undefined-reference-labels      | 26        | default  |
| MDS053 | no-unused-link-definitions         | 21        | default  |
| MDS036 | max-section-length                 | 20        | opt-in   |
| MDS001 | line-length                        | 19        | default  |
| MDS023 | paragraph-readability              | 19        | default  |
| MDS063 | descriptive-link-text              | 19        | opt-in   |
| MDS024 | paragraph-structure (this fixture) | 18        | opt-in   |
| MDS062 | link-validity                      | 15        | default  |

The parity-gap profile on the real mdsmith repo points
the same direction. End-to-end `mdsmith check .` takes
504 ms, of which the heaviest default-only rule, MDS020
required-structure, is 19% of CPU. The cost is
dominated by per-host-file re-parses of the schema
file and per-file recompiles of the schema's CUE
expression. Both are caches the existing
[`lint.RunCache`][runcache] already proves the shape
for (front matter, includes).

[runcache]: ../internal/lint/runcache.go

## Approach

One gate, many fixes. The gate is the same `cold File
minus parse baseline` measurement MDS024's
[bench_test.go][mds024gate] uses; lifted into
[`internal/integration/`](../internal/integration/) so it
runs against every rule the production set registers via
`internal/rules/all`. Each rule is a sub-test, so a
failure names the rule and leaves the rest visible.

[mds024gate]: ../internal/rules/paragraphstructure/bench_test.go

Per-rule fixes follow plan 193's pattern. Trace each
rule's allocation profile with pprof and
`b.ReportAllocs`. Identify the hot allocator (regex over
source, re-collected map, repeated parse). Replace it
with the cheapest equivalent — a memoized helper, a
package-scope regex, a byte scan, a slice instead of a
map, or a reusable buffer.

The parity-gap fix lifts schema parsing and CUE compile
into RunCache. Each schema file is then parsed once per
`engine.Run`. Each unique schema CUE expression is
compiled once, regardless of how many host files
reference it.

## Tasks

1. [x] Add the parametric per-rule gate at
   `internal/integration/alloc_budget_test.go`. Add
   `race_off_test.go` and `race_on_test.go` as the
   build-tag pair. The gate then skips cleanly
   under `-race`.
2. [x] Fix MDS026 table-readability (18 → 10).
   `tableRow.cells` is now `[][]byte` sub-slices
   into the source line, so the per-row cost is one
   slice header instead of N cell strings. The
   readability counters consume them via
   `bytes.TrimSpace` + `utf8.RuneCount` + a new
   `countWords` byte-scanner that mirrors
   `len(strings.Fields(string(b)))` without the
   intermediate string. `stripPrefix` also loses
   its `string(line) … []byte(s[len:])` round-trip
   on the empty- and matching-prefix paths.
   `columnHeader` converts to string lazily for the
   one diagnostic message that needs it. Grandfather
   entry removed.
3. [🔳] MDS025 table-format stays at 105 allocs/op
   (grandfathered). The format pass parses cells as
   strings; the structure pass (MD055/056/058)
   re-parses every row independently. Reducing to
   ≤ 10 needs the single-table-walk refactor
   scheduled as a follow-up to plan 181 — one walk
   that produces both shapes. The grandfather row
   in `internal/integration/alloc_budget_test.go`
   pins today's number so a regression fails CI.
4. [x] Fix MDS001 line-length (19 → ≤ 10). Dropped the
   three empty `map[int]bool{}` literals in
   buildCategories. Replaced the per-line
   `tableLineRe.Match` with isTableLineStart. Replaced
   the per-long-line `urlOnlyRe.MatchString` with
   isURLOnlyLine. Built the diagnostic message via
   strconv.Itoa + concat instead of fmt.Sprintf.
5. [x] Fix MDS027 cross-file-reference-integrity
   (25 → 7). Defers `linkgraph.CollectAnchors(self)`
   and the per-Check `anchorCache` map until the
   first link that actually needs them. The gate
   fixture's one cross-file `[other](other.md)` link
   has no anchor, so both stay nil. Splits
   `checkRelativeTarget` into a cheap `targetExists`
   path that skips the heap-escaping read closure in
   `resolveTargetFile` when the link is not a
   Markdown target with an anchor. Adds `cachedAbs`
   to `fscache.go` so the per-Check `resolveAbsRoot`
   calls become a sync.Map hit after the first call.
6. [x] Fix MDS053 no-unused-link-definitions (11 → 9).
   The Check path now splits on `len(defs)`: the
   single-def branch (the universal case for the
   typical file) skips the usedLabels map entirely
   and short-circuits the AST walk via a new
   `isLabelUsedInAST` helper that returns true on
   the first matching reference. The multi-def
   branch keeps the previous `seen` + lazy
   `usedLabels` shape unchanged. Grandfather entry
   removed.
7. [x] Fix MDS054 no-undefined-reference-labels (13 → 8).
   `collectNormalisedDefs` returns a sized
   `[]string` instead of a `map[string]bool`; the
   slice header + N strings replaces the map
   header + bucket + N strings + an insert grow on
   the gate fixture's 1-ref case. The new
   `labelDefined` linear-scans the slice — N is the
   per-file refdef count, which is small in
   practice. A no-bracket early-exit
   (`bytes.ContainsRune(f.Source, '[')`) skips the
   entire helper allocation for prose files. The
   `len(r.Placeholders) > 0 &&` guard on every
   `placeholders.ContainsBodyToken(string(label), …)`
   call avoids the per-match `string(label)` cast
   when no placeholder vocabulary is configured
   (the default). Grandfather entry removed; new
   `alloc_test.go` in the rule package pins the
   number under the ≤ 10 ceiling.
8. [x] Fix MDS062 link-validity to ≤ 10 allocs. The
   plan 195 engine-bench chunk inlined
   `LineOfOffset`'s binary search and the
   message-string cache. On the current gate fixture
   MDS062 lands at 6 allocs.
9. [x] Fix MDS063 descriptive-link-text to ≤ 10 allocs.
   The per-File `MDS063.bannedSet` memo paid a
   ~13-alloc build every Check (four normalised
   banned phrases plus map setup). The cache now
   lives on the Rule instance behind an
   `atomic.Pointer[map]` plus `sync.Mutex` double-
   checked-lock, so the build runs once per
   configured rule. ApplySettings clears the pointer
   so a reconfigured Banned list rebuilds on the
   next read. Current alloc count: 4.

10. [x] Fix MDS023 paragraph-readability to ≤ 10 allocs.
    The plan-195 engine-bench chunk (LineOfOffset
    inlined binary search, message-string cache, slot
    value semantics) dropped MDS023 to 10/Check on the
    gate fixture.
11. [x] Fix MDS024 paragraph-structure on the
    representative fixture to ≤ 10 allocs. Same chunk
    as MDS023 dropped it to 10/Check.
12. [x] Fix MDS036 max-section-length to ≤ 10 allocs.
    The configured-no-knobs path (every limit zero,
    no per-level / per-heading override) now returns
    nil before walking the AST for headings or
    paragraphs. The opt-in default ships with every
    knob zero, so the alloc-budget gate's reading is
    0 allocs/Check. The paragraph index also only
    builds when at least one paragraph-aware limit is
    set, so the line-only configuration skips the
    paragraph walk.
13. [x] Fix MDS029 conciseness-scoring to ≤ 10 allocs.
    The classifier's regex-driven cue extraction paid
    ~400 allocs every Check it ran. A cheap byte-scan
    word count now gates the classifier call. So
    paragraphs below `MinWords` never enter the
    classifier. MDS029 drops to 2 allocs on the gate
    fixture's single sub-MinWords paragraph.
14. [x] Fix MDS035 toc-directive to ≤ 10 allocs.
    The rule's `hasTOCLinkReference` helper re-parsed
    the entire source with `lint.NewParser()` on every
    fresh File to consult goldmark's link-reference
    table; the per-File memo wrapper hid the cost but
    each new File still paid for one full parse (~200
    allocs). Switching to `f.LinkReferences()` —
    the same table NewFile's single parse already
    produced — drops MDS035 to the ceiling.
15. [x] Closed the MDS020 schema-parse parity gap via
    two new RunCache slots: `ParsedSchema(absPath,
    build)` and `CompiledCUE(source, build)`. Each
    caches once per `engine.Run`. MDS020's parseSchema
    in Check, Fix, and bodySync, plus
    `schema.ValidateFrontmatterDiags`, all route
    through the cache. `mdsmith check .` drops from
    ~490 ms to ~460 ms; `BenchmarkCheckCorpusLarge`
    p95 stays flat (the corpus has no schemas). PR
    #377 follow-ups close the LSP cache-invalidation
    gaps Copilot flagged.
16. [x] Re-run `BenchmarkCheckCorpusLarge` to confirm
    no engine-corpus regression. Latest run lands at
    p95 = 188 ms / 314 µs per file — well under the
    plan's 12 s p95 acceptance criterion. The
    `BenchmarkParityGap` measurement was a local
    one-off used while sizing the schema-cache work
    (task 15) and is intentionally not committed.
17. [x] Update [docs/development/index.md][budget] to
    point at the new gate as the enforcement point.

## Results

The gate runs as `go test -run=TestPerRuleAllocBudget
./internal/integration/`. The first failing run is the
baseline above; the row count drops as each task lands.

The fixture and `allocsForRule` helper live in
[alloc_budget_test.go][gate]. The same file ships
`BenchmarkPerRuleAllocBudget`, which prints every
rule's headroom in one table so a contributor can spot
rules close to the budget before they cross it.

[gate]: ../internal/integration/alloc_budget_test.go

## Risk

Per-rule fixes for the cross-file/link family
(MDS027, MDS053, MDS054, MDS062, MDS063) touch shared
helpers in [`internal/linkgraph/`](../internal/linkgraph/)
and [`internal/lint/`](../internal/lint/). A regression
in any of those would surface in the existing fixture
tests in `internal/rules/MDS###-*/` and the LSP
contract tests in
[`internal/lsp/`](../internal/lsp/). Land each rule's
fix as its own commit so a bisect names the change.

The MDS020 schema cache is the highest-blast-radius
change. The schema parse path runs through CUE; a
stale cache would silently let a schema rewrite go
unnoticed by every host file. Mitigation: keyed by
absolute path and CUE source string, no time-based
invalidation, scoped to one `engine.Run` lifetime —
matches RunCache's existing Includes and FrontMatter
slots.

## Acceptance Criteria

- [x] `TestPerRuleAllocBudget` passes for every
      registered rule. MDS025 is grandfathered at
      105 with a documented follow-up to plan 181;
      every other rule fits the ≤ 10 ceiling.
- [x] `BenchmarkPerRuleAllocBudget` lists every rule
      at ≤ 10 allocs/op except MDS025 (105,
      grandfathered as above).
- [x] `BenchmarkCheckCorpusLarge` stays within its
      existing 12 s p95 budget. Latest p95 = 222 ms.
- [x] Each fix has a unit test pinning the new
      behaviour.
- [x] `mdsmith check .` passes.
- [x] `go test ./...` and `go test -race ./...` pass.
- [x] `go tool golangci-lint run` reports no issues.
