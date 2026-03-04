---
id: 67
title: Custom ProcessingInstruction AST Node
status: ✅
---
# Custom ProcessingInstruction AST Node

## Goal

Add a custom goldmark AST node type for processing instructions.
This enables AST-based marker search in `FindMarkerPairs` and
clean type checks in rule code.

## Context

Plan 66 switched directives to `<?name?>` syntax. Goldmark
parses both PIs and regular HTML as `ast.HTMLBlock`. String
heuristics distinguish them today. A custom block parser
intercepts `<?...?>` before `HTMLBlockParser`, giving clean
type-based distinction. `FindMarkerPairs` can then walk the
AST instead of scanning raw lines.

## Tasks

### 1. New AST node: `internal/lint/pi.go`

```go
type ProcessingInstruction struct {
    ast.BaseBlock
    ClosureLine text.Segment
    Name        string // directive name from <?name
}
```

- `KindProcessingInstruction = ast.NewNodeKind("ProcessingInstruction")`
- `IsRaw()` returns true, `HasClosure()` mirrors `ast.HTMLBlock`
- Import `"github.com/yuin/goldmark/text"` (no alias — matches
  existing usage in `file.go`)
- `Name` is the substring between `<?` and the first whitespace
  or `?>`, whichever comes first (e.g. `"catalog"`, `"/include"`,
  `"allow-empty-section"`). Must be non-empty; empty names are
  rejected by the parser (`Open` returns `nil`)
- `Lines()` contains the opening line (`<?name...`) and any
  YAML body lines. `ClosureLine` contains only the `?>`
  terminator and is NOT included in `Lines()`. For single-line
  PIs (e.g. `<?foo?>`), `Lines()` has one entry and
  `ClosureLine` points to the same segment

### 2. New block parser: `internal/lint/pi_parser.go`

- Trigger: `[]byte{'<'}`
- `Open`: match `^[ ]{0,3}<?`, extract `Name`, create node.
  Return `nil` immediately for lines starting with `<` but not
  `<?` so HTMLBlockParser handles them (both parsers share the
  `<` trigger byte).
  Also return `nil` when `parent` is not the document root
  (`parent.Kind() != ast.KindDocument`), so PIs inside
  blockquotes, lists, and other container blocks are rejected.
  Return `nil` when the extracted name is empty (e.g. `<??>`,
  `<? ?>`).
  For single-line PIs where the line ends with `?>` (e.g.
  `<?foo?>`): set `ClosureLine` to the same segment, return
  `(node, parser.Close | parser.NoChildren)` — `Continue` is
  never called
- `Continue`: close when the trimmed line equals `?>` (set
  `ClosureLine`). Use exact match after `strings.TrimSpace`,
  consistent with current `trimmed == terminator` behavior in
  `processMarkerLine`.
  If EOF is reached without `?>`, goldmark calls `Close`
  automatically; the node is created with
  `HasClosure() == false`
- `Close`: no-op
- `CanInterruptParagraph`: true (matches HTML block behavior)
- `CanAcceptIndentedLine`: false
- Priority **850** (before `HTMLBlockParser` at 900)
- Code block parsers at 500-700 claim their lines first, so PI
  markers inside code blocks are naturally excluded

### 3. Register parser: `internal/lint/file.go`

Replace `goldmark.DefaultParser()` with a custom parser that adds
`NewPIBlockParser()` at priority 850. Preserve all default
parsers and transformers:

```go
p := parser.NewParser(
    parser.WithBlockParsers(
        append(parser.DefaultBlockParsers(),
            util.Prioritized(NewPIBlockParser(), 850),
        )...,
    ),
    parser.WithInlineParsers(
        parser.DefaultInlineParsers()...,
    ),
    parser.WithParagraphTransformers(
        parser.DefaultParagraphTransformers()...,
    ),
)
```

### 4. Rewrite marker search

File: [`gensection/parse.go`](../internal/archetype/gensection/parse.go)

Replace raw-line scanning with AST walk:

```go
func FindMarkerPairs(
    f *lint.File, directiveName, ruleID, ruleName string,
) ([]MarkerPair, []lint.Diagnostic)
```

Walk only top-level children of `f.AST` (skip nodes nested
inside blockquotes or lists). Match
`*lint.ProcessingInstruction` nodes:

- `pi.Name == directiveName` -> start marker; extract YAML body
  from `pi.Lines()` (lines 2..N, skipping `<?name` first line)
- `pi.Name == "/"+directiveName` -> end marker; close pair
- Nested/orphaned/unclosed markers -> diagnostics
- Unterminated PI (`HasClosure() == false`) used as start marker
  -> emit "unclosed marker" diagnostic

Derive `MarkerPair` line numbers from AST nodes:

- `StartLine`: `f.LineOfOffset(startPI.Lines().At(0).Start)`
- `ContentFrom`: line after the start PI node's last line
  (after `ClosureLine` for multi-line PIs, after the single
  line for single-line PIs)
- `EndLine`: `f.LineOfOffset(endPI.Lines().At(0).Start)`
- `ContentTo`: `EndLine - 1`

Delete (no longer needed):

- `CollectIgnoredLines`, `addHTMLBlockLines`, `addBlockLineRange`
- `processMarkerLine`, `processLineInsidePair`,
  `processLineOutsidePair`
- `IsDirectiveBlock`, `IsSingleLineDirective`
- `MarkerPair.FirstLine` (set but never read by any consumer;
  `ParseDirective` uses `YAMLBody`, not `FirstLine`)

### 5. Simplify engine

File: [`gensection/engine.go`](../internal/archetype/gensection/engine.go)

Remove `startPrefix`, `endMarker`, `terminator` fields from
`Engine`. `NewEngine` just stores the directive.

Updated call sites:

```go
func (e *Engine) Check(f *lint.File) []lint.Diagnostic {
    pairs, diags := FindMarkerPairs(
        f, e.directive.Name(),
        e.directive.RuleID(), e.directive.RuleName(),
    )
    // ...
}
```

`Fix` follows the same pattern.

### 6. Update rule

File: [`emptysectionbody/rule.go`](../internal/rules/emptysectionbody/rule.go)

- `hasAllowMarker`: check `*lint.ProcessingInstruction` with
  `pi.Name == markerName`
- `hasMeaningfulContent`: add
  `case *lint.ProcessingInstruction: continue`, remove
  `IsDirectiveBlock` call from HTMLBlock case
- Remove `gensection` import

### 7. Tests

New `internal/lint/pi_test.go` — parser unit tests:

Basic parsing:

- `<?foo?>` -> `*ProcessingInstruction`, `Name == "foo"`
- `<?foo\nbar\n?>` -> `Name == "foo"`, `HasClosure() == true`
- `<?foo\n?>` -> multi-line with empty YAML body
- `<?/include?>` -> `Name == "/include"`
- `<!-- comment -->`, `<div>` -> still `*ast.HTMLBlock`

Edge cases:

- `<?foo?>` inside a fenced code block -> no PI node produced
- `    <?foo?>` (4-space indent) -> not a PI (indented code
  block takes precedence)
- ` <?foo?>`, `  <?foo?>`, `   <?foo?>` (1-3 spaces) -> PI
  node produced
- `<?foo\nbar` (unterminated, no `?>` before EOF) -> node
  created with `HasClosure() == false`, no panic
- Multiple PIs in one document -> each produces its own node
- PI after a paragraph line -> interrupts paragraph
  (`CanInterruptParagraph` is true)
- `<?foo\n   \n?>` -> whitespace-only YAML body
- `> <?foo?>` inside a blockquote -> no PI node (container
  blocks strip the prefix before the block parser sees the
  line; the `Open` method must only accept lines whose parent
  is the document root so that directives nested inside
  blockquotes or lists are not treated as PIs)
- `<??>`, `<? ?>` (empty name) -> no PI node, falls through
  to HTMLBlockParser
- `<?foo?>\n<?bar?>` (consecutive PIs without blank line) ->
  two separate PI nodes
- `<?foo?>` single-line closes in `Open` (never calls
  `Continue`); verify `HasClosure() == true`

`FindMarkerPairs` integration tests
(`gensection/parse_test.go`):

- Full start/end pair -> correct `StartLine`, `ContentFrom`,
  `ContentTo`, `EndLine`, `YAMLBody` values
- YAML body extracted from `Lines()[1:]`, excluding opening
  line and `ClosureLine`
- Unterminated start PI -> "unclosed marker" diagnostic

Update [`engine_test.go`](../internal/archetype/gensection/engine_test.go):

- Update `FindMarkerPairs` calls to new signature

## Acceptance Criteria

- [ ] `<?...?>` lines produce `*lint.ProcessingInstruction` nodes
      in the AST (not `*ast.HTMLBlock`)
- [ ] `FindMarkerPairs` walks the AST instead of scanning raw lines
- [ ] `emptysectionbody` uses type checks, not string heuristics
- [ ] `IsDirectiveBlock` and `IsSingleLineDirective` are deleted
- [ ] `CollectIgnoredLines` and related line-scanning helpers are
      deleted
- [ ] `go test ./...` passes
- [ ] `mdsmith check .` exits 0
- [ ] `golangci-lint run` reports no issues
