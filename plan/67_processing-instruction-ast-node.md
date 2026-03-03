---
id: 67
title: Custom ProcessingInstruction AST Node
status: 🔲
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
    ClosureLine textm.Segment
    Name        string // directive name from <?name
}
```

- `KindProcessingInstruction = ast.NewNodeKind("ProcessingInstruction")`
- `IsRaw()` returns true, `HasClosure()` mirrors `ast.HTMLBlock`
- `Name` extracted during parsing (e.g. `"catalog"`, `"/include"`,
  `"allow-empty-section"`)

### 2. New block parser: `internal/lint/pi_parser.go`

- Trigger: `[]byte{'<'}`
- Open: match `^[ ]{0,3}<?`, extract `Name`, create node
- Continue: close when line contains `?>` (set `ClosureLine`)
- Priority **850** (before `HTMLBlockParser` at 900)
- Code block parsers at 500-700 claim their lines first, so PI
  markers inside code blocks are naturally excluded

### 3. Register parser: `internal/lint/file.go`

Replace `goldmark.DefaultParser()` with custom parser that appends
`NewPIBlockParser()` at priority 850 to
`parser.DefaultBlockParsers()`.

### 4. Rewrite marker search

File: [`gensection/parse.go`](../internal/archetype/gensection/parse.go)

Replace raw-line scanning with AST walk:

```go
func FindMarkerPairs(
    f *lint.File, directiveName, ruleID, ruleName string,
) ([]MarkerPair, []lint.Diagnostic)
```

Walk `f.AST` for `*lint.ProcessingInstruction` nodes:

- `pi.Name == directiveName` -> start marker; extract YAML body
  from `pi.Lines()` (lines 2..N, skipping `<?name` first line)
- `pi.Name == "/"+directiveName` -> end marker; close pair
- Nested/orphaned/unclosed markers -> diagnostics

Delete (no longer needed):

- `CollectIgnoredLines`, `addHTMLBlockLines`, `addBlockLineRange`
- `processMarkerLine`, `processLineInsidePair`,
  `processLineOutsidePair`
- `IsDirectiveBlock`, `IsSingleLineDirective`

### 5. Simplify engine

File: [`gensection/engine.go`](../internal/archetype/gensection/engine.go)

Remove `startPrefix`, `endMarker`, `terminator` fields from
`Engine`. `NewEngine` just stores the directive. `Check`/`Fix`
call `FindMarkerPairs(f, e.directive.Name(), ...)`.

### 6. Update rule

File: [`emptysectionbody/rule.go`](../internal/rules/emptysectionbody/rule.go)

- `hasAllowMarker`: check `*lint.ProcessingInstruction` with
  `pi.Name == markerName`
- `hasMeaningfulContent`: add
  `case *lint.ProcessingInstruction: continue`, remove
  `IsDirectiveBlock` call from HTMLBlock case
- Remove `gensection` import

### 7. Tests

New `internal/lint/pi_test.go`:

- `<?foo?>` -> `*ProcessingInstruction`, `Name == "foo"`
- `<?foo\nbar\n?>` -> `Name == "foo"`, `HasClosure() == true`
- `<?/include?>` -> `Name == "/include"`
- `<!-- comment -->`, `<div>` -> still `*ast.HTMLBlock`

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
