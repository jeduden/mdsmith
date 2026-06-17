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
model: sonnet
depends-on: [2606171258]
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

MDS003 and MDS004 carry a placeholder setting. A placeholder match
needs the heading text, which is inline. When the placeholder list is
empty (the parity case) that branch is dead, so the line-only path is
exact. Guard the nil-AST path so it only claims Layer 0 when the
placeholder list is empty; otherwise it stays on the AST.

## Tasks

For each rule:

1. Add the nil-AST path — a `CheckBlock` for the per-heading rules, or
   a span/line walk in `Check` for the document-order rules (MDS003,
   MDS051). Reuse the existing level/position extraction.
2. Keep the diagnostic byte-identical: same line, column, message.
3. Regenerate the walk audit (`MDSMITH_REGEN_WALK_AUDIT=1`) and sync
   the embedded [rulelayer copy](../internal/rulelayer/rule_walk_audit.json).
4. Add a `TestCheck_NilASTMatchesAST` unit test, the shape MDS002 uses.

## Acceptance Criteria

- [ ] Each rule resolves to Layer 0 (audit `A-no-skipping`).
- [ ] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with them on.
- [ ] `go test ./...` passes.
