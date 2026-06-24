---
id: 2606240211
title: Add dedicated unit tests for locate.go helpers
status: "🔲"
model: sonnet
summary: >-
  internal/index/locate.go has 12 unexported
  helpers without dedicated unit tests. Add a
  named test for each so the audit policy is
  satisfied.
---
# Add dedicated unit tests for locate.go helpers

## Goal

Add a named unit test for each of the 12 unexported
helpers in `internal/index/locate.go`. The 2026-06-24
audit requires it.

## Background

Go arch doc §"Tests" requires every production function
to have a dedicated test by name. The 2026-06-24 audit
(range: 1599c9f..09f22d3) flagged this file.

Functions without a dedicated test as of 09f22d3:

- `headingInfo` — extract text, level, anchor from
  a heading node
- `locateInAST` — walk AST at a line/col offset
- `linkContainsOffset` — byte offset inside a link
- `linkCloseOffset` — find closing bracket offset
- `scanForByte` — scan source for a target byte
- `linkToLocate` — project `*ast.Link` to result
- `piToLocate` — project PI node to result
- `listItemValue` — parse `- key: value` line
- `headingOnLine` — find heading at a given line
- `frontMatterListItem` — parse front-matter row
- `frontMatterParentKey` — walk up to parent key
- `offsetAt` — convert (line, col) to byte offset

`isGlobPattern` is a trivial one-liner with no branch.
Add a "// no test by design" exemption comment.

## Tasks

1. For each of the 12 functions above, add at least
   one `TestFunctionName` in
   `internal/index/locate_test.go`. Drive the helper
   directly, not through `Locate`.
2. Add a `// no test by design` comment on
   `isGlobPattern` in `internal/index/locate.go`.
3. `go test ./internal/index/...` passes.
4. `go vet ./internal/index/...` passes.

## Acceptance Criteria

- [ ] `locate_test.go` contains `TestheadingInfo`,
      `TestlocateInAST`, `TestlinkContainsOffset`,
      `TestlinkCloseOffset`, `TestscanForByte`,
      `TestlinkToLocate`, `TestpiToLocate`,
      `TestlistItemValue`, `TestheadingOnLine`,
      `TestfrontMatterListItem`,
      `TestfrontMatterParentKey`, `TestoffsetAt`.
- [ ] `isGlobPattern` carries a "// no test by
      design" comment.
- [ ] `go test ./internal/index/...` is green.
- [ ] `mdsmith check .` is green.
