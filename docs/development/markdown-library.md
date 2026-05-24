---
title: Public Markdown Library
summary: >-
  The pkg/markdown public package: parse,
  produce, and its compatibility policy.
---
# Public Markdown Library

`github.com/jeduden/mdsmith/pkg/markdown` is
mdsmith's one importable Markdown surface. It
owns the single goldmark parser config in the
repository plus a byte-exact producer. The
linter core, the release tooling, and external
callers all share it.

It depends only on goldmark and the standard
library. No code here imports `internal/lint` or
any `internal/` package. The dependency points
the other way.

The goldmark dependency itself is a vendored
fork at `pkg/goldmark/`. A `replace` directive
in the root `go.mod` wires it in. Plan 197 reuses
one `text.BlockReader` per parser. Plan 198 adds
a per-parse slab arena absorbing the four
structural allocators (text, paragraph,
segments, segments backing).

A `parser.WithNoArena()` option and the
`goldmark_upstream` build tag both turn the arena
off. The equivalence harness in `pkg/goldmark/`
and the `goldmark-fork-test` CI job diff both
paths.

## Why it exists

Plan 163 extracted this package. Two problems
forced it.

First, `internal/release/syncdocs.go` had grown
its own raw `goldmark.New()` parser. That was
the interim from PR #291. It lifted doc titles
and stripped directive markers for the Hugo
site. A second parser config drifts from the
linter's.

Second, the parser config and the `<?…?>`
processing-instruction block parser lived in
`internal/lint`. Every reuse coupled the caller
to the linter core.

The config now lives here once. `internal/lint`
re-exports the symbols (see
[below](#relationship-to-internallint)). Its
~150 callers compile unchanged. Exactly one
goldmark config exists in the tree.

## How this differs from goldmark

`pkg/markdown` configures goldmark's parser. It
does not replace or fork it. The differences
that matter:

- **Processing instructions.** A `<?name … ?>`
  block parses to a `ProcessingInstruction`
  node. Stock goldmark parses it as a
  CommonMark type-3 raw HTML block.
- **No renderer.** goldmark renders an AST to
  HTML. This package has no renderer. The
  producer is `Splice`: byte-exact span surgery
  on the original source. mdsmith never
  re-renders Markdown.
- **Front-matter split.** `StripFrontMatter` is
  a byte split, not goldmark's frontmatter
  extension. The closing `---` is matched only
  at the start of a line. So a `---` row inside
  a YAML block scalar does not end it early.
- **CommonMark only.** The canonical parser
  enables no GFM extensions. Tables,
  strikethrough, and the rest are composed
  separately by the MDS034 flavor detector.
- **Byte-stability contract.** `Splice` output
  is pinned byte-for-byte (see the policy
  below). goldmark gives no such cross-version
  guarantee for rendered output.

## Parse API

`Parse` is the high-level entry. It splits YAML
front matter off the source. It parses the body
with the canonical parser:

```go
doc := markdown.Parse(source)
// doc.FrontMatter — raw prefix incl. --- fences, or nil
// doc.Body        — source with the prefix removed
// doc.AST          — goldmark AST of doc.Body
```

`Parse` never errors and never panics. Empty,
body-only, and front-matter-only inputs each
yield a document node.

`ParseContext(src, ctx)` is the lower-level
entry. It parses `src` verbatim — no
front-matter handling. It uses the pooled
canonical parser. It records link-reference
definitions in the supplied `parser.Context`.
Use it when you need that context. The linter
file model does, to read link references
without a second parse.

`NewParser()` returns a fresh canonical
`parser.Parser`. That is the default CommonMark
block, inline, and paragraph parsers plus the
processing-instruction block. Callers that add
extensions on top start from this. The MDS034
flavor detector is one. The PI handling stays
consistent that way.

`StripFrontMatter(source)` and `CountLines(b)`
are the front-matter split primitives. The
closing `---` fence is matched only at the start
of a line. So a YAML block scalar containing a
`---` row does not truncate the front matter.

### The processing-instruction node

A `<?name … ?>` marker parses to a
`*markdown.ProcessingInstruction` block node,
not a raw HTML block. It exposes `Name`,
`Lines()`, `ClosureLine`, and `HasClosure()`.
The same syntax inside a fenced code block or
inline code span is structurally distinct. So
directive documentation parses as code and is
left verbatim.

`KindProcessingInstruction` is the registered
`ast.NodeKind`. `NewPIBlockParser()` and
`PIBlockParserPrioritized()` register it on a
parser.

## Produce API

mdsmith has no AST-to-Markdown renderer.
`mdsmith fix` is edit-based: it applies per-rule
segment edits. The producer matches that model.

```go
out := markdown.Splice(body, []markdown.Edit{
    {Start: a, End: b}, // half-open byte ranges,
    {Start: c, End: d}, // ascending, non-overlapping
})
```

`Splice` returns a new slice. It equals `body`
with the edit ranges removed in one
left-to-right pass. It does not mutate `body`.
The output is byte-exact span surgery on the
original source, not a re-render. So it never
fights `mdsmith fix`.

An AST walk over a parsed `Document` yields
heading and processing-instruction spans in
document order. That is already the order
`Splice` expects.

`internal/release/syncdocs.go` is the reference
consumer. It lifts the first body H1 to a
front-matter `title:`. It strips directive
markers by collecting their spans and calling
`Splice`.

## Relationship to internal/lint

`internal/lint` consumes this package. It
re-exports the parse surface so the linter's
callers need not import `pkg/markdown` directly:

- `lint.ProcessingInstruction` is a type alias
  for `markdown.ProcessingInstruction`, so
  existing type switches keep working.
- `lint.KindProcessingInstruction` is the same
  registered kind value.
- `lint.NewParser`, `lint.NewPIBlockParser`,
  `lint.PIBlockParserPrioritized`,
  `lint.StripFrontMatter`, and `lint.CountLines`
  forward here.
- `lint.NewFile` parses via
  `markdown.ParseContext`.

The lint-owned YAML decoders
(`ParseFrontMatterKinds`,
`ParseFrontMatterFields`) stay in
`internal/lint`. They are linter concerns, not
parsing.

`internal/mdtext` still owns the AST *walk*
helpers: slugging, TOC collection, plain-text
extraction. It walks a node it is handed.
`pkg/markdown` produces that node. The two
answer different questions.

## Compatibility policy

`pkg/markdown` is a cross-system public surface
(see
[cross-system contracts](architecture/cross-system.md)).
mdsmith is at major 0. Strict SemVer does not
bind yet. Breaks must be deliberate and noted in
the changelog.

The stable surface:

- `Parse` and `Document` (its fields).
- `ParseContext`, `NewParser`.
- `StripFrontMatter`, `CountLines`.
- `Splice` and `Edit` (its fields).
- `ProcessingInstruction` (its exported fields
  and methods), `KindProcessingInstruction`,
  `NewPIBlockParser`, `PIBlockParserPrioritized`.

Policy:

- Adding a function, type, or field is a minor,
  additive change.
- Renaming or removing an exported symbol is a
  break. So is changing a function signature.
  That is major post-1.0, changelog-noted
  pre-1.0.
- The goldmark AST shape `Parse` returns tracks
  the goldmark dependency. A goldmark major
  upgrade that changes node types is a break for
  this surface. It is called out in the
  changelog.
- The `<?…?>` marker grammar is a shipped
  [generated-section](../background/concepts/generated-section.md)
  contract. Once parsed a certain way, it keeps
  parsing that way until the next major. A
  deprecation diagnostic may precede removal.
- `Splice` output is byte-exact. For the same
  input it must keep producing the same bytes.
  This is pinned by the `TestSplice` cases here
  and, at the consumer, by the
  `TestReconcileDocForHugo_*` table tests in
  `internal/release/syncdocs_test.go`.
