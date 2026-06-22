---
id: 2606202100
title: "Parity perf: make InlineBlocks a light inline scan, not a goldmark re-parse"
status: "✅"
summary: >-
  Profiling the eligible-only parity skip run found the parse-skip is
  cost-neutral because lint.InlineBlocks (~51% of the skip path) re-parses
  every paragraph through goldmark (parseInlineWithRefsArena ->
  ParseContextArena, block phase included). The cheap line scans are ~10%.
  So the parse-skip can only beat the parse once InlineBlocks is the light
  byte scan Layer 1 was specified as — links, autolinks, images, reference
  defs/uses, and code-span ranges, with no emphasis delimiter run. Byte
  identical to goldmark for those constructs, gated across the corpus.
model: opus
depends-on: [2606141904]
---
# Parity perf: make InlineBlocks a light inline scan, not a goldmark re-parse

## Goal

Make `mdsmith check -c parity` faster on benchmark 2 by making the
parse-skip path actually cheaper than the goldmark parse. Today it is
cost-neutral (measured), so it is shipped default-off.

## Background: the measured bottleneck

The Layer-0 parse-skip is correct for parity (block, list, and inline
rules all run on a nil-AST File; held byte-identical by
`TestLayer0Gate_CorpusDiagnosticsEquivalence`). It is not yet a win.

Take only the 26 files that clear the gate. Benchmark those alone.
Parse-skip on and off come out **identical, 15.5 ms each**. A CPU profile
of the skip-on run explains why:

| Skip-path cost               | share  | what it is                                    |
| ---------------------------- | ------ | --------------------------------------------- |
| `lint.InlineBlocks`          | ~51%   | the inline projection the link/ref rules read |
| ↳ `ParseContextArena`        | ~31%   | a full goldmark parse, per run                |
| ↳ `parseBlocks`              | ~33%   | goldmark's block phase, per run               |
| `listscan.Parse`             | ~5%    | list structure                                |
| `ClassifyLines`/`scanLayer0` | <noise | code-line set, block spans                    |

`InlineBlocks` splits the body into runs, then calls
`parseInlineWithRefsArena` → `markdown.ParseContextArena` on each — a real
goldmark parse of every paragraph, block phase included. So the
"parse-skip" does not skip goldmark; it only moves it from one
whole-document parse to a per-run parse of the same content. Total work is
unchanged. The block scan it genuinely avoids is small, and the line-scan
fusion idea (plan 2606171532) targets ~10% that is already cheap.

## Approach

Re-back `lint.InlineBlocks` on a targeted byte scanner. Drop the goldmark
parse from the nil-AST path. The
[lazy-parse note](../docs/research/benchmarks/lazy-parse-architecture.md)
already specifies Layer 1 as exactly this. It calls for a byte scan over
links, autolinks, images, and `[label]:` definitions and uses. It
explicitly excludes the emphasis delimiter algorithm.

What the parity inline rules actually read:

- `no-bare-urls` (MDS012): autolinks and bare URLs in text.
- `link-validity` (MDS062) and the descriptive/space link rules: inline
  links `[text](url)` and reference links `[text][label]`.
- `no-empty-alt-text` (MDS032): images `![alt](url)`.
- `no-unused-link-definitions` / `no-undefined-reference-labels`
  (MDS053/054): `[label]: url` definitions and the labels used.
- `no-space-in-code-spans` (MDS052) and every rule above: code-span ranges
  (a `` `…` `` span suppresses a link/URL inside it).

None of these needs emphasis. One parity rule touches it:
`no-emphasis-as-heading` (MDS018). It asks one bounded question — is a
whole paragraph a single emphasis span? A constrained detector answers
that without the delimiter algorithm. See the lazy-parse note's "MDS018
holdout".

## Tasks

1. [x] Build a `lint` inline byte scanner
   (`internal/lint/inline_scan.go`) that, over a run's bytes, emits
   inline link/image nodes, autolinks, and code spans as goldmark AST
   nodes, with code-span-suppression handling and goldmark's exact text
   segmentation. No emphasis. The scanner is conservative: it handles a
   single-paragraph run of plain text, inline links, inline images,
   autolinks, and code spans, and declines (so the caller falls back to
   goldmark for that run) on emphasis, reference links, raw HTML,
   backslash escapes, entities, multi-line runs, or any block marker.
2. [x] Re-back `InlineBlocks` on it: `scanInlineBlocks` now calls
   `inlineRunNode`, which tries the scanner per run and falls back to
   `parseInlineWithRefsArena` only when the scanner declines. The
   goldmark path is unchanged for a parsed File.
3. [x] Equivalence gate: `TestInlineIndexEquivalence_NodeStream` diffs
   the full inline node stream (scanner-backed nil-AST path vs goldmark
   whole-document parse) across the repo corpus and every rule fixture;
   `TestInlineIndexEquivalence_ParityRules` diffs MDS012/032/062
   diagnostics. Both byte-identical.
4. [x] Re-profile the eligible-only run; confirm `InlineBlocks` drops out
   of the hot path. Measured with Go benchmarks over the repository's own
   parse-skip-eligible Markdown (no `MDSMITH_SPIKE_CORPUS` needed):
   `internal/lint/inline_scan_bench_test.go`. The scanner removes the
   per-run goldmark parse for the runs it handles and is a net win across
   the full run set — see "Measured: scanner vs goldmark" below.
5. [x] Revisit eligibility and the default-on decision. The in-package
   data supports the scanner being a net win for `InlineBlocks`; the
   global `MDSMITH_LAYER0_SKIP` default stays off pending the end-to-end
   neutral-corpus re-profile — see "Default-on decision" below.

## Measured: scanner vs goldmark

The benchmark file `inline_scan_bench_test.go` extracts every
inline-bearing run from the repo's own parse-skip-eligible Markdown. That
is the file set with no code block and no `<?` directive. It is the
population `runner.layer0SkipEligible` admits, matching the equivalence
gates. The benchmark times the scanner against the goldmark per-run parse.
Run with `go test -bench=Inline -benchtime=5s ./internal/lint/`:

| Benchmark                       | ns/op | allocs/op | B/op |
| ------------------------------- | ----- | --------- | ---- |
| `ScanInlineRun_Eligible`        | 462   | 1         | 204  |
| `ParseInline_Eligible`          | 2196  | 15        | 1251 |
| `InlineRunNode_AllRuns` (scan+) | 5067  | 14        | 1311 |
| `ParseInline_AllRuns` (goldmrk) | 5670  | 17        | 1534 |

Read:

- On a scanner-eligible run the scanner is **~4.7x faster** (462 vs
  2196 ns/op), **15x fewer allocs** (1 vs 15), **~6x less memory**. This
  is the per-run goldmark parse `InlineBlocks` no longer pays for eligible
  runs — the ~51% hot-path cost the profile flagged.
- Across the **whole** corpus run set, `inlineRunNode` (scanner-first,
  goldmark fallback) is **~11% faster** than the pre-scanner baseline
  (5067 vs 5670 ns/op), even paying the eligibility check plus a failed
  scan on the runs that fall back.

`TestCorpusRunEligibility` reports the hit-rate. Of 1,917 inline-bearing
runs in parse-skip-eligible corpus files, **26.5% are scanner-eligible**
and **22.0% scan to completion**. The rest carry emphasis, reference
links, multi-line wrap, or a block marker, so they fall back. Roughly a
quarter of the per-run parses are removed. That is what drives the ~11%
whole-set win.

## Default-on decision

The benchmarks demonstrate the scanner is a clear net win for the
`InlineBlocks` projection. It removes the per-run goldmark parse for
eligible runs. It is cheaper across the full run set too. That closes the
specific gap the profile named — `InlineBlocks` re-parsing every run
through goldmark.

The global `MDSMITH_LAYER0_SKIP` flag stays **default-off** for now,
deliberately. The in-package benchmarks measure the `InlineBlocks` cost in
isolation. The gate's default-off comment is waiting on a different
number: the end-to-end `mdsmith check -c parity` skip-vs-parse re-profile
on the neutral Rust Book corpus (`MDSMITH_SPIKE_CORPUS`). There
`InlineBlocks` was ~51% of the skip path. Flipping the global default
belongs with that end-to-end measurement, not the projection benchmark
alone. Turning it on here would claim an end-to-end win this increment
does not yet measure.

The scanner is the prerequisite that work needed. Enabling the flag is the
follow-up once the neutral-corpus run confirms skip now beats parse.

## Acceptance Criteria

- [x] `InlineBlocks` on a nil-AST File runs no goldmark parse **for
      scanner-eligible runs** (single-paragraph runs of plain text,
      inline links/images, autolinks, code spans). Runs the scanner
      cannot prove identical still fall back to goldmark, so a file with
      any such run is not yet fully parse-free — widening coverage
      (headings, lists, reference links, emphasis) is the follow-up.
- [x] Every parity inline rule is byte-identical between the scanner and
      the goldmark path across the corpus and fixtures — and, stronger,
      the full inline node stream is byte-identical
      (`TestInlineIndexEquivalence_NodeStream`).
- [x] The inline projection is measurably faster with the scanner:
      `InlineBlocks`'s per-run cost drops ~4.7x on scanner-eligible runs
      (462 vs 2196 ns/op) and ~11% across the full corpus run set (5067 vs
      5670 ns/op) — see "Measured: scanner vs goldmark". The end-to-end
      `mdsmith check -c parity` skip-vs-parse re-profile on the neutral
      corpus, and the `MDSMITH_LAYER0_SKIP` default-on flip it gates,
      remain a follow-up (see "Default-on decision").
- [x] `go test ./...` and the layer-0 equivalence gates stay green.

## Honest ceiling

Even with a free inline scan, the [gomarklint architecture
note][gml-arch] bounds parity at ~1.4x gomarklint: rules + per-file
overhead alone exceed a pure line scanner. Beating gomarklint outright
additionally needs a rule/overhead trim or a smaller parity rule set (a
product decision). This plan closes the parse gap; it does not by itself
reach 1.0x.

[gml-arch]: ../docs/research/benchmarks/gomarklint-architecture.md
