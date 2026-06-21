---
id: 2606202100
title: "Parity perf: make InlineBlocks a light inline scan, not a goldmark re-parse"
status: "🔲"
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

1. Build a `lint` inline byte scanner that, over a run's bytes, emits:
   link/image spans (inline and reference form), autolink/bare-URL spans,
   reference definitions, and code-span ranges — with backslash-escape and
   code-span-suppression handling. No emphasis.
2. Re-back `InlineBlocks` / `WalkInlineNodes` (or the specific projections
   the inline rules consume) on it when `f.AST == nil`; keep the goldmark
   path for the parsed File.
3. Equivalence gate: diff every parity inline rule's output (scanner vs
   goldmark) across the neutral corpus, the repo corpus, and every rule
   fixture. Byte-identical or it does not ship — the same discipline the
   block scan met.
4. Re-profile the eligible-only run; confirm `InlineBlocks` drops out of
   the hot path and the skip beats the parse.
5. Only then revisit eligibility (block-quote descent) and the
   default-on decision.

## Acceptance Criteria

- [ ] `InlineBlocks` on a nil-AST File runs no goldmark parse.
- [ ] Every parity inline rule is byte-identical between the scanner and
      the goldmark path across the corpus and fixtures.
- [ ] The eligible-only parity skip run is measurably faster than the
      parse run (it is a wash today).
- [ ] `go test ./...` and the layer-0 equivalence gates stay green.

## Honest ceiling

Even with a free inline scan, the [gomarklint architecture
note][gml-arch] bounds parity at ~1.4x gomarklint: rules + per-file
overhead alone exceed a pure line scanner. Beating gomarklint outright
additionally needs a rule/overhead trim or a smaller parity rule set (a
product decision). This plan closes the parse gap; it does not by itself
reach 1.0x.

[gml-arch]: ../docs/research/benchmarks/gomarklint-architecture.md
