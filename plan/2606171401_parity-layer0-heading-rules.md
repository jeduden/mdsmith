---
id: 2606171401
title: "Parity parse-skip: migrate the Layer-0 heading and front-matter rules"
status: "🔲"
summary: >-
  Add a nil-AST path to the parity rules that read only a heading's
  line or the front matter — MDS003 heading-increment, MDS004
  first-line-heading, MDS051 single-h1, MDS069 unique-frontmatter — so
  each resolves to Layer 0 and stops forcing the goldmark parse, gated
  byte-identical to the AST path across the corpus.
model: opus
depends-on: [2606171258, 2606171400]
---
# Parity parse-skip: migrate the Layer-0 heading and front-matter rules

## Goal

Move the heading-shape and front-matter parity rules onto the Layer-0
block scan, so they no longer force the parse.

## Background

[MDS002 heading-style](../internal/rules/headingstyle/rule.go) is the
proven template. It reads a heading's style and level from the heading
line and serves the nil-AST path through a `rule.BlockChecker`
`CheckBlock`, byte-identical to its AST `CheckNode`.

The rules below read only line-level facts: a heading's level
(leading-`#` run or setext underline), its position, or front-matter
keys. None needs the inline tree.

| Rule   | Name               | Reads                     |
| ------ | ------------------ | ------------------------- |
| MDS003 | heading-increment  | heading levels, in order  |
| MDS004 | first-line-heading | first block kind + level  |
| MDS051 | single-h1          | count of level-1 headings |
| MDS069 | unique-frontmatter | front-matter keys         |

## Implementation notes (found while scoping)

MDS003 and MDS004 are **config-dependent**, not statically Layer 0. They
carry a placeholder setting; a placeholder match reads the heading text,
which is inline. With an empty placeholder list (the parity case) that
branch is dead and the line-only path is exact. This is the same shape
as line-length, so they use the config-aware gate from
[2606171400](2606171400_parity-gate-unification.md): implement
`rule.LineCapable` returning `len(Placeholders) == 0`, plus a nil-AST
`Check` that walks heading lines from the block scan. The static
`rulelayer` audit cannot express that config dependence, so do **not**
mark them `A-no-skipping`. This is why the plan now depends on 2606171400.

MDS051 single-h1 is opt-in. It reads only level-1 heading counts plus a
front-matter title. The same `LineCapable`-when-simple treatment fits.

MDS069 unique-frontmatter never touches `f.AST`. Its `Check` builds a
cross-file front-matter index. The walk audit reports it
`inconclusive-not-fired`: the fixture probe has no include globs, so the
rule never emits. The audit cannot confirm nil-AST safety from a probe
that never fires. The fix is an audit probe that fires (an include glob
over a multi-file fixture) or an explicit nil-AST-safe classification.
It is not a `CheckBlock` migration.

## Tasks

For each rule:

1. Add the nil-AST `Check` path that walks the Layer 0 heading spans
   (levels, position), reusing the existing extraction.
2. Implement `rule.LineCapable` returning true only for the simple
   config (empty placeholders for MDS003/004), so the 2606171400 gate
   admits the rule under parity and forces the parse otherwise.
3. Keep the diagnostic byte-identical: same line, column, message.
4. Add a `TestCheck_NilASTMatchesAST` unit test, the shape MDS002 uses,
   plus the gate-level skip + corpus-equivalence guards 2606171400 added.

## Acceptance Criteria

- [ ] MDS003, MDS004, MDS051 skip the parse under parity (empty
      placeholders) and force it otherwise, byte-identical both ways.
- [ ] MDS069's nil-AST safety is resolved (audit probe or classification).
- [ ] `TestLayer0Gate_LineCapableCorpusEquivalence`-style guard green
      with these rules on.
- [ ] `go test ./...` passes.
