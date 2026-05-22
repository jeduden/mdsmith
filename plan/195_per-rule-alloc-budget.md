---
id: 195
title: Enforce the ≤ 10 allocs/op per-rule budget across every registered rule
status: "🔳"
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

1. [x] Add `internal/integration/alloc_budget_test.go`
   (the parametric per-rule gate) plus the
   `race_off_test.go` / `race_on_test.go` build-tag pair
   that lets the gate skip cleanly under `-race`.
2. [🔳] Partial fix for MDS026 table-readability (37 →
   23 on the initial gate fixture; further engine-bench
   cuts in the same PR brought the current grandfather
   baseline to 18 — see
   `internal/integration/alloc_budget_test.go` for the
   authoritative number). Lands the early-exit pair
   (no-pipe-in-source, no-pipe-on-line) and the
   byte-scanner detectPrefix + splitRow. Remaining
   ≥10-alloc budget needs the cells-as-byte-offsets
   refactor (tableRow stores `source []byte` +
   `cellRanges []int` rather than `[]string`); deferred
   so the cell-storage move and the rule-coverage_test
   updates land together.
3. [🔳] Partial fix for MDS025 table-format (63 → 55 on
   the initial gate fixture; current grandfather
   baseline 50 — see
   `internal/integration/alloc_budget_test.go`). Lands
   the same early-exit pair through the tableformat
   rule and `tablefmt.findTables`. Same `cells []string`
   refactor blocks the rest.
4. [x] Fix MDS001 line-length (19 → ≤ 10). Dropped the
   three empty `map[int]bool{}` literals in
   buildCategories, replaced the per-line
   `tableLineRe.Match` with isTableLineStart, replaced
   the per-long-line `urlOnlyRe.MatchString` with
   isURLOnlyLine, and built the diagnostic message via
   strconv.Itoa + concat instead of fmt.Sprintf.
5. [ ] Fix MDS027 cross-file-reference-integrity:
   memoize `linkgraph.CollectAnchors(self)` and
   `linkgraph.ExtractLinks(f)` on the `lint.File` so the
   per-Check rebuild does not pay the AST walk again
   when MDS053/MDS054 already triggered it; drop the
   per-link `anchorCache` map literal when the file has
   no link targets to check.
6. [ ] Fix MDS053 no-unused-link-definitions to ≤ 10
   allocs. The fixture's `[ref]:` defines a label;
   plan 188's inventory flags the per-file regex
   over `f.Source` as the hot allocator.
7. [ ] Fix MDS054 no-undefined-reference-labels to ≤ 10
   allocs. Same regex-over-source pattern as MDS053;
   the inventory entry pairs them.
8. [ ] Fix MDS062 link-validity to ≤ 10 allocs. Profile
   notes `computeLineStarts` as a hot helper — move it
   to a per-File memo so successive link checks share
   one line-index instead of rebuilding it per link.
9. [ ] Fix MDS063 descriptive-link-text to ≤ 10 allocs.
10. [ ] Fix MDS023 paragraph-readability to ≤ 10 allocs.
    The rule already consumes the memoized
    `astutil.CollectSectionParagraphs`; the residual
    allocation is in `mdtext.CountWords` and the
    diagnostic message build. Pool the word counter's
    state or skip the count when below the minimum-word
    floor.
11. [ ] Fix MDS024 paragraph-structure on the
    representative fixture to ≤ 10 allocs. Its own
    abbr-heavy fixture lands at 9; the representative
    fixture adds a heading, a code fence, a list, and a
    table, all of which create extra paragraphs that
    inflate the cold-File cost. Tighten the shared
    walk's per-non-prose-block skip.
12. [ ] Fix MDS036 max-section-length to ≤ 10 allocs.
13. [ ] Fix MDS029 conciseness-scoring to ≤ 10 allocs.
14. [ ] Fix MDS035 toc-directive to ≤ 10 allocs.
15. [ ] Close the MDS020 schema-parse parity gap. Add a
    `RunCache.ParsedSchema(absPath, build)` slot that
    parses each schema file once per `engine.Run`, and
    a `RunCache.CompiledCUE(srcKey, build)` slot that
    compiles each unique schema CUE expression once.
    The MDS020 hot path (parseSchema + CompileString)
    drops from per-host-file to per-schema-source.
16. [ ] Re-run `BenchmarkCheckCorpusLarge` and the new
    `BenchmarkParityGap` (one-off, removed before
    merge) to confirm the default-vs-parity gap closes
    on the engine corpus.
17. [ ] Update [docs/development/index.md][budget] to
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

- [ ] `TestPerRuleAllocBudget` passes for every
      registered rule (all sub-tests green).
- [ ] `BenchmarkPerRuleAllocBudget` lists every rule
      at ≤ 10 allocs/op.
- [ ] `BenchmarkCheckCorpusLarge` stays within its
      existing 12 s p95 budget.
- [ ] Each fix has a unit test pinning the new
      behaviour (test pyramid: unit + the integration
      gate).
- [ ] `mdsmith check .` passes.
- [ ] `go test ./...` and `go test -race ./...` pass.
- [ ] `go tool golangci-lint run` reports no issues.
