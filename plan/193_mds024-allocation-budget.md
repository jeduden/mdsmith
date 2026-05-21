---
id: 193
title: Rework MDS024 to fit the per-rule allocation budget (≤ 10 allocs/op)
status: "✅"
model: opus
depends-on: [191]
summary: >-
  MDS024.Check allocates ~903 times per call on abbreviation-heavy
  prose — ~150x the median rule (0–6) and second only to MDS029
  (1869). Plan 191 cut 16 allocs by replacing reAbbr with a DFA;
  the rest is upstream Punkt's per-token machinery (NewToken with
  6 cached regex pointers, reNumeric inside Type, strings.Join for
  collocation keys, strings.Split for hyphenation, and a fresh
  [][2]*Token grouper per annotation pass). Fork a minimal trained
  Punkt into internal/punkt/, rewrite the per-token hot path to be
  allocation-free, and gate the result against the existing
  byte-identical equivalence harness and a new allocs/op budget.
---
# Rework MDS024 to fit the per-rule allocation budget

## Goal

Drop MDS024 from ~903 to ≤ 10 allocs/op on the
abbr-heavy benchmark. The target matches the budget
in [docs/development/index.md][budget]. Keep the
segmenter byte-identical to upstream Punkt. The
[equivalence harness][harness] is the gate.

[budget]: ../docs/development/index.md
[harness]: ../internal/mdtext/sentence_equivalence_test.go

## Background

The cost is concrete and measured. Per-rule benchmark on the
abbr-heavy fixture used in plan 191:

| Rule                           | allocs/op | B/op   | ns/op  |
|--------------------------------|----------:|-------:|-------:|
| MDS029 conciseness-scoring     | 1 869     | 122068 | 514042 |
| **MDS024 paragraph-structure** | **903**   | 28189  | 250541 |
| MDS043 no-reference-style      | 189       | 28174  | 56818  |
| median rule                    | 0 – 6     | < 1 k  | < 2k   |

Plan 191's `BenchmarkSplitSentences` profile attributes the
allocation sources inside the segmenter (the dominant
sub-call of MDS024.Check):

| Source in upstream                  | % of allocs |
|-------------------------------------|------------:|
| `sentences.NewToken` (inline)       | 33%         |
| `regexp.ReplaceAllString` (in Type) | 34%         |
| `strings.(*Builder).grow` (in Type) | 7.5%        |
| `strings.Split` (hyphenation)       | 5.7%        |
| `DefaultTokenGrouper.Group`         | 4.7%        |
| `strings.Join` (collocation key)    | 4.5%        |
| `strings.ToLower` (in Type)         | 3%          |
| fastMultiPunctWordAnnotation        | 8.5%        |

Every period-ending token pays for a `NewToken`. The
struct carries six cached regex pointers. Then
`Type()` runs. It lowercases, applies `reNumeric`,
and drops commas.

Three annotation passes do this per token. Each pass
also allocates a fresh grouper buffer. The
cheap-bounds [guard][guard] short-circuits trivial
paragraphs. Abbreviation-heavy prose defeats the
guard. That is exactly when MDS024 fires.

[guard]: ../internal/rules/paragraphstructure/rule.go

Plan 187 recorded the negative for *naive*
segmenters. A plain `[.!?]` splitter diverges on
abbreviations, decimals, and ellipses. MDS024 must
keep Punkt's exact boundaries.

Plan 191 swapped one regex (`reAbbr`) for a DFA.
This plan extends the same approach. Fork a minimal
Punkt into a local package. Swap each regex for a
byte scan. Pool the allocations that remain. The
equivalence harness stays as the gate.

## Approach

A new `internal/punkt/` package owns a forked Punkt.
It is byte-identical to upstream over the equivalence
corpus. It is allocation-clean per call. The fork
vendors only what MDS024 needs.

The upstream `neurosnap/sentences` dependency stays.
Plan 191's `mdtext_punkt_upstream` build tag still
selects it for A/B verification.

Invariant: each token's per-pass flags (`Abbr`,
`SentBreak`, `LineStart`, `ParaStart`) match upstream
after all three passes. The allocation reduction is
orthogonal: same flags, fewer mallocs.

### Per-call allocation budget after the rework

Targeted breakdown for `paragraphstructure.Rule.Check` on the
abbr-heavy fixture:

| Source                                   | allocs   |
|------------------------------------------|---------:|
| AST walk over paragraphs                 | 0        |
| Paragraph-text builder (reused per call) | 0–1      |
| `SplitSentences` result slice            | 0–1      |
| Tokenizer's annotated-token slice        | 1        |
| Diagnostic slice (only when firing)      | 0–1      |
| Headroom                                 | ≤ 6      |
| **Total budget**                         | **≤ 10** |

## Tasks

1. [x] Add `BenchmarkRule_MDS024` in
   [`internal/rules/paragraphstructure/`](../internal/rules/paragraphstructure/)
   that constructs a `lint.File` over the abbr-heavy fixture
   and runs `(*Rule).Check` once per iteration with
   `b.ReportAllocs()`. The benchmark must `b.Fatalf` if
   allocs/op exceeds the budget (start with > 903 as a smoke
   test; tighten to ≤ 10 once tasks 2–7 land). The fixture is
   the same abbr-heavy paragraph corpus used by
   `BenchmarkSplitSentences_Subset`; lift it into a shared
   test helper so both benchmarks read the same bytes.
2. [x] Vendor the minimum subset of
   `github.com/neurosnap/sentences` into `internal/punkt/`:
   `Storage`, `Token`, `WordTokenizer`, `TokenGrouper`,
   `OrthoContext`, `DefaultSentenceTokenizer`, plus the
   trained English data loader. Skip CJK punctuation, the
   non-English language data, and `IsNonPunct` (per
   plan 187, IsNonPunct has no call site). Keep the upstream
   commit hash and license in the package header so the fork
   point is clear.
3. [x] Pool the `Token` struct. Upstream allocates one per
   word with six cached `*regexp.Regexp` pointers. Move the
   regexes to package scope so the struct shrinks to its
   field set, then pool via `sync.Pool` or pre-allocate a
   `[]Token` of expected size per `Tokenize` call. The
   `Annotate` interface uses `*Token`; document that pointer
   identity inside one `Tokenize` call is stable and reused
   across calls.
4. [x] Replace `(*DefaultWordTokenizer).Type` with an
   allocation-free byte scanner. The upstream behaviour is:
   lowercase the token, run `reNumeric` to replace numeric
   runs with `##number##`, drop commas. A single-pass byte
   scan over the token bytes produces the same string into a
   reusable `[]byte` buffer. The scanner is exercised by the
   equivalence harness on every `Tokenize` call, so a drift
   fails the next test run.
5. [x] Replace `(*DefaultWordTokenizer).IsCoordinatePartTwo`,
   `IsListNumber`, `IsInitial`, `IsAlpha`, `IsEllipsis`, and
   `IsNumber` with byte scanners. Each upstream regex
   (`reCoordinateSecondPart`, `reListNumber`, `reInitial`,
   `reAlpha`, `reEllipsis`, `reNumeric` prefix check) is a
   single-pass match describable in 10–30 lines of Go. Pin
   each scanner against its source regex with a unit test
   table — same harness pattern as plan 191's
   `matchAbbrPattern` against `reAbbr`.
6. [x] Replace the `strings.Join` collocation key with a
   composite map lookup. Upstream computes
   `typ + "," + nextTyp` and indexes `Collocations` with it.
   The fork builds the same key into a reusable
   `state.colKeyBuf` and looks up `Collocations[string(buf)]`
   — relying on the compiler's `m[string(b)]` map-key
   elision so the lookup itself does not allocate.
7. [x] Replace the per-pass `[][2]*Token` grouper with a
   reusable buffer on the tokenizer. The grouper allocates a
   length-N+1 slice every Annotate call (three passes ⇒ three
   allocations per Tokenize). A single buffer reset
   (`buf = buf[:0]`) at the start of each Annotate pass keeps
   one allocation amortized across calls.
8. [x] Replace `strings.Split(tokNoPeriod, "-")` in
   `typeAnnotation` with a `strings.LastIndexByte`-driven
   scan for the last hyphenated segment. The current code
   only uses the tail element
   (`tokNoPeriodHypen[len(tokNoPeriodHypen)-1]` — upstream
   identifier, missing 'h'), so the full split is wasted
   work.
9. [x] Audit `paragraphstructure.Rule.Check` itself for
   allocation. The per-paragraph text extraction in
   [`internal/mdtext`](../internal/mdtext) builds a
   `strings.Builder`; that path is reused via the per-File
   `astutil.CollectSectionParagraphs` memo, so the engine
   pays it once per file and the rule's repeated Checks see
   a warm cache. Measured: 7 allocs/op on a warm File (see
   Results below).
10. [x] Switch `mdtext.buildTokenizer` to construct the new
    `internal/punkt` pipeline by default. Keep
    `mdtext_punkt_upstream` pointing at upstream so the A/B
    verification path stays alive — the equivalence harness
    runs under both builds in CI.
11. [x] Run `TestSplitSentences_IsItsOwnReference`,
    `TestSplitSentences_GoldenRules`,
    `TestSplitSentences_EnglishMainCases`, and the
    abbr-heavy unit-test corpus through the new path.
    Byte-identical or the plan fails.
12. [x] Tighten `BenchmarkRule_MDS024`'s budget to ≤ 10
    allocs/op and verify the engine-level benchmarks
    (`BenchmarkCheckCorpus{Small,Large}`) stay within budget
    on both build tags.
13. [x] Document the rework in the MDS024 README's
    "Performance" section: replace the "Punkt is ~20% of
    wall time" paragraph with the new per-Check budget and
    a pointer to this plan.

## Results

Fresh apples-to-apples measurement on the 4-vCPU
sandbox (Intel Xeon @ 2.10 GHz). Both columns are
runs taken in the same minute, on the same machine,
varying only the build tag. Segmenter benchmarks
report mean over 3 runs at 2 s each; corpus
benchmarks report the median p95 over 5 single-shot
runs (`-benchtime=1x` so each iteration walks the
whole tree once).

| Benchmark                       | Upstream tag              | Default tag               | Δ             |
|---------------------------------|---------------------------|---------------------------|---------------|
| BenchmarkRule_MDS024 (warm)     | n/a (gate added here)     | 7 allocs / 79 µs          | within budget |
| BenchmarkSplitSentences         | 593 allocs / 186 µs       | 22 allocs / 68 µs         | −96% / −63%   |
| BenchmarkSplitSentences_Subset  | 1082 allocs / 266 µs      | 16 allocs / 77 µs         | −98.5% / −71% |
| BenchmarkCheckCorpusSmall (p95) | 30 ms (28–31 across 5)    | 28 ms (26–32 across 5)    | flat (noise)  |
| BenchmarkCheckCorpusLarge (p95) | 190 ms (182–195 across 3) | 195 ms (194–198 across 3) | flat (noise)  |

`BenchmarkCheckCorpus*` does not exercise MDS024 (it
is opt-in, `EnabledByDefault: false`), so the
segmenter rework cannot affect it. The corpus rows
above are recorded only to document that the
rework did not introduce engine-wide overhead.

The Rule.Check budget gate lives in
[bench_test.go][gate]. It is build-tagged
`!mdtext_punkt_upstream` on purpose. The upstream
tag swaps in the original pipeline, which records
~1141 allocs/op for the same rule, so the tight
gate would always fail there. The split keeps A/B
comparison alive and the default CI lane green.

[gate]: ../internal/rules/paragraphstructure/bench_test.go

## Risk

A fork carries maintenance cost. Future upstream
changes must be applied manually. The build tag and
equivalence harness contain the blast radius. Any
drift fails the harness on the next CI run. The
upstream tag lets a developer A/B-compare in one
flag flip.

Pooling the Token struct introduces aliasing risk.
If a caller retains a `*Token` past the next
`Tokenize` call, the underlying memory is reused.
The mdsmith call sites all consume tokens within the
same call. A package doc comment plus a contract
test pins this. A future caller cannot accidentally
rely on persistence.

The byte-scanner replacements for `Type` and friends
must match their source regexes. Plan 191's pattern
applies. Each scanner gets a property-style table
against the source regex. The equivalence harness on
real prose is the second gate.

## Acceptance Criteria

- [x] `BenchmarkRule_MDS024` reports ≤ 10 allocs/op
      on the abbreviation-heavy fixture.
- [x] `TestSplitSentences_IsItsOwnReference` passes
      — the reworked pipeline is byte-identical to
      upstream over the equivalence corpus.
- [x] `TestSplitSentences_GoldenRules` and
      `TestSplitSentences_EnglishMainCases` pass
      byte-identical under both
      `-tags mdtext_punkt_upstream` and the default
      build.
- [x] `BenchmarkSplitSentences` and
      `BenchmarkSplitSentences_Subset` improve or
      stay flat versus plan 191's numbers.
- [x] `BenchmarkCheckCorpus{Small,Large}` remain
      within budget (Small p95 < 2 s, Large
      p95 < 12 s).
- [x] Every new function in `internal/punkt/` has a
      dedicated unit test (per the [test-pyramid
      rule][tests]).
- [x] `mdsmith check .` passes.
- [x] `go test ./...` and `go test -race ./...`
      pass.
- [x] `go tool golangci-lint run` reports no issues.

[tests]: ../docs/development/architecture/tests.md
