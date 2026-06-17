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
as line-length, so they ride the config-aware gate from
[2606171400](2606171400_parity-gate-unification.md), whose
`ruleConfiguredLineCapable` admits a rule when its configured instance
reports `rule.LineCapable`. The static `rulelayer` audit cannot express
that config dependence, so they cannot be marked `A-no-skipping`. This is
why the plan now depends on 2606171400.

**Caveat on `LineCapable`.** Its doc comment commits the interface to the
flat line classifier — "reading only `f.Lines` and the classifier-backed
projections", not `Layer0(f).BlockSpans`. A heading rule that reads
heading *levels* from block spans therefore does not fit the contract as
written. Resolve this before coding, one of two ways. Option one widens
the `rule.LineCapable` doc to admit the block-span projection. Option two
adds a config-aware *block* eligibility to the gate, gating a
`BlockChecker`'s skip on `len(Placeholders) == 0` so these rules stay
`BlockChecker`s like MDS002 (cleaner; prefer it).

MDS051 single-h1 is opt-in. It reads only level-1 heading counts plus a
front-matter title — no placeholder branch — so it is unconditionally
nil-AST-safe, not "when-simple". It can take the static `BlockChecker`
path (like MDS002), no config gate needed.

MDS069 unique-frontmatter never touches `f.AST`. Its `Check` builds a
cross-file front-matter index. The walk audit reports it
`inconclusive-not-fired`: the fixture probe has no include globs, so the
rule never emits. The audit cannot confirm nil-AST safety from a probe
that never fires. The fix is an audit probe that fires (an include glob
over a multi-file fixture) or an explicit nil-AST-safe classification.
It is not a `CheckBlock` migration.

## Blocker: the scanner emits no nested heading spans

`scanLayer0` ([layer0.go](../internal/lint/layer0.go)) emits
`BlockATXHeading` / `BlockSetextHeading` spans only for **top-level**
headings. It emits one `BlockList` span per list line with no descent
into the item body, and one `BlockQuote` span that maps back only
`CodeBlockLines`, never the inner scan's heading spans. The AST path
(`ast.Walk`) visits headings nested in list items and blockquotes too.

So a nil-AST path that walks heading spans **diverges** from the AST for
any document with a heading inside a list or quote. A missed nested
heading is a false negative. It also corrupts MDS003's `prevLevel`
sequence for every later heading, and MDS004/MDS051 miscount. The
repository corpus has few such headings, so the equivalence gate would
not reliably catch the divergence.

This must be fixed first. One option extends `scanLayer0` to emit nested
heading spans in `ast.Walk` order, recursing into list-item and
blockquote bodies and mapping their spans back the way `CodeBlockLines`
already is. The other has the gate force the parse for any document whose
headings are container-nested. Until then these rules cannot migrate.
Note also that the `BlockSpans` doc in layer0.go still says they have "no
production consumer yet" — stale once MDS010/011/031 land; update it.

## Tasks

For each rule:

1. First resolve the nested-heading blocker above (scanner emits nested
   heading spans, or the gate excludes container-nested headings).
2. Add the nil-AST `Check` path that walks the Layer 0 heading spans
   (levels, position), reusing the existing extraction.
3. Wire config-aware skip eligibility (the `LineCapable` caveat above:
   widen the doc, or add a block-eligibility gate) for MDS003/MDS004.
   MDS051 takes the static `BlockChecker` path, no config gate.
4. Keep the diagnostic byte-identical: same line, column, message.
5. Add a `TestCheck_NilASTMatchesAST` unit test, the shape MDS002 uses,
   with a heading inside a list and inside a blockquote, plus the
   gate-level skip + corpus-equivalence guards 2606171400 added.

## Acceptance Criteria

- [ ] The nested-heading blocker is resolved: a heading inside a list or
      blockquote yields identical diagnostics on the parse-skip and
      full-parse paths (scanner extension or gate exclusion).
- [ ] MDS003, MDS004 skip the parse under parity (empty placeholders) and
      force it otherwise, byte-identical both ways; MDS051 via the static
      `BlockChecker` path.
- [ ] MDS069's nil-AST safety is resolved (audit probe or classification).
- [ ] `TestLayer0Gate_LineCapableCorpusEquivalence`-style guard green
      with these rules on.
- [ ] `go test ./...` passes.
