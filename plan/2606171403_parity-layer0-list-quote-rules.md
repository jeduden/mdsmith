---
id: 2606171403
title: "Parity parse-skip: migrate the Layer-0 list and blockquote rules"
status: "✅"
summary: >-
  Add a nil-AST path to the parity rules that read list and blockquote
  structure — MDS014 blank-line-around-lists, MDS016 list-indent,
  MDS045 list-marker-style, MDS046 ordered-list-numbering, MDS061
  list-marker-space, MDS059 blockquote-whitespace, MDS067 callout-type
  — each gated byte-identical to the AST across the corpus.
model: opus
depends-on: [2606171258]
---
# Parity parse-skip: migrate the Layer-0 list and blockquote rules

## Goal

Move the list and blockquote parity rules onto the Layer-0 block scan,
so they no longer force the parse.

## Background

List and quote spans are the subtle case. CommonMark list parsing
folds in loose vs tight lists, nesting, and lazy continuation, so a
Layer-0 span boundary can drift from the AST. The
[Layer-0 scanner](../internal/lint/layer0.go) emits one `BlockList`
span per list-item run and one `BlockQuote` span, and descends into
their bodies. There is no per-item span kind; a rule that needs
item-level granularity walks the list span's body lines itself.

Each rule needs the marker shape (`-` / `*` / `+`, or `1.` / `1)`),
the marker indent, and the inter-item spacing — all line-level facts.
MDS067 callout-type reads only a blockquote's first line, so it lives
here too. Verify the span boundaries match the AST before trusting them.

| Rule   | Name                    | Reads                     |
| ------ | ----------------------- | ------------------------- |
| MDS014 | blank-line-around-lists | top-level list span edges |
| MDS016 | list-indent             | item marker indent        |
| MDS045 | list-marker-style       | bullet marker character   |
| MDS046 | ordered-list-numbering  | ordered marker sequence   |
| MDS061 | list-marker-space       | spaces after the marker   |
| MDS059 | blockquote-whitespace   | `>` marker spacing        |
| MDS067 | callout-type            | blockquote first line     |

## Tasks

For each rule:

1. First confirm the relevant span boundary matches the AST across the
   corpus. Add the nil-AST path only once it does.
2. Add a `CheckBlock` (or span walk) reusing the existing extraction.
   Keep the diagnostic byte-identical: same line, column, message.
3. Regenerate the walk audit and sync the embedded
   [rulelayer copy](../internal/rulelayer/rule_walk_audit.json).
4. Add a `TestCheck_NilASTMatchesAST` unit test covering nested lists,
   a loose list, and a list that holds a code fence.

## Acceptance Criteria

- [x] Each rule resolves to Layer 0 (audit `A-no-skipping`).
- [x] `TestLayer0Gate_CorpusDiagnosticsEquivalence` green with them on.
- [x] `go test ./...` passes.

## Implementation notes

The seven rules now branch in `Check`. A parsed File takes the AST path.
A nil-AST File (`f.AST == nil`) takes a Layer-0 path. The blockquote
rules walk the scanner's `BlockQuote` spans directly. MDS059 detects the
MD028 blank-line gap between adjacent spans; MDS067 reads the `[!type]`
token off each span's first line.

The four list rules share a new line-based list parser,
`internal/rules/listscan`. It re-derives goldmark's list facts from
`f.Lines`. The facts are: the item marker line, the nesting level, the
list ordered-ness and `Start`, the per-item literal number, the
multi-block flag, and the top-level list's first and last line. The block
scanner cannot supply these. It collapses a list into one single-line
`BlockList` span per marker, with no nesting.

`listscan.Parse` is checked byte-for-byte against the goldmark AST in
`listscan_ast_test.go`. The test corpus spans the rules' bad fixtures
plus nested, loose, and code-fence cases.

Each rule extracts a shared `verdict`/`itemVerdict` helper that both
paths drive, so the two paths agree by construction. A
`TestCheck_NilASTMatchesAST` per rule pins it on nested lists, a loose
list, and a list holding a code fence.

`listscan` does not handle tab-indented lists, lists nested inside block
quotes, or HTML-block list interruption. The parse-skip gate still forces
an AST parse for any source containing a list or a `>`
(`layer0SkipEligible` conditions 4 and 5). So the nil-AST path for these
rules runs only in the audit and unit tests today. Relaxing those gate
conditions is the dependent benchmark work, not this plan.
