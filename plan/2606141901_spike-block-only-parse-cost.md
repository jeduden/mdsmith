---
id: 2606141901
title: "Spike: block-only parse cost vs gomarklint"
status: "✅"
summary: >-
  Measure whether a block-only parse (no inline, no tree)
  plus the parity structural rules plus per-file overhead
  beats gomarklint on benchmark 2. This number gates the
  whole lazy-parse rearchitecture: if rules + overhead
  alone already exceed gomarklint, Layer 0 is not enough
  and the plan must also trim them.
model: opus
depends-on: []
---
# Spike: block-only parse cost vs gomarklint

## Goal

Answer the open question in the
[lazy-parse architecture research][research]. Is
`Layer 0 (block scan) + parity structural rules +
overhead` faster than gomarklint on the neutral corpus?
Everything downstream rests on that answer. Measure it
before rearchitecting the parser.

## Background

The [gomarklint study][gomarklint] set out the
arithmetic. Parity splits into parse ~38%, rules ~42%,
and overhead ~19%. Even a *free* parse leaves rules plus
overhead near gomarklint's time. A block-only parse is
the cheapest proxy for Layer 0. goldmark already runs
block and inline parsing as separate phases. So a
throwaway measurement can suppress the inline phase
without a rewrite.

This is a spike: the code it produces is a measurement
harness, not shipped behaviour. It lives behind a flag
or on a scratch branch and is discarded once the number
is recorded.

## Tasks

1. Add a parse path that runs block parsing only —
   suppress the inline walk and skip building inline
   nodes (`parser.WithBlockOnly`, behind the default-off
   `engine.Runner.BlockOnlyParse`). Reach it from an
   in-process benchmark, and — to get an apples-to-apples
   CLI wall-time number against gomarklint — from an
   off-by-default `MDSMITH_SPIKE_BLOCK_ONLY` env gate too.
2. Time three things on the pinned neutral corpus:
   block-only parse alone, block-only parse + the
   parity structural rules, and the full parity run,
   each against gomarklint.
3. Record the numbers and the go/no-go reading in the
   [research doc][research] "honest performance bar"
   section.

## Acceptance Criteria

- [x] A measured comparison of block-only parse + parity
      structural rules + overhead against gomarklint on
      benchmark 2, captured in the research doc.
- [x] An explicit go/no-go: does Layer 0 alone clear the
      bar, or must rule/overhead trimming ride along?
      (No — block-only is ~1.70x gomarklint; rules +
      overhead alone already exceed it, so trimming must
      ride along.)
- [x] No production code path changes; the harness is
      flagged off or discarded. (`BlockOnlyParse` and the
      `MDSMITH_SPIKE_BLOCK_ONLY` gate are default-off; the
      benchmark is gated on `MDSMITH_SPIKE_CORPUS`.)

[research]: ../docs/research/benchmarks/lazy-parse-architecture.md
[gomarklint]: ../docs/research/benchmarks/gomarklint-architecture.md
