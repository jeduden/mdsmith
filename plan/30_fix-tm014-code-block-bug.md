# Fix TM014 Fixer Corrupting Code Blocks

## Goal

Fix a bug where TM014 (blank-line-around-lists) inserts blank
lines inside fenced code blocks, corrupting file content. Add
defensive code block awareness to TM013 as well.

## Context

Running `tidymark fix` on markdown files with fenced code
blocks inside numbered list items corrupts the files. TM014
(blank-line-around-lists) inserts blank lines inside fenced
code blocks because it checks/fixes raw `f.Lines` adjacent to
AST List nodes without verifying those lines aren't inside
code blocks.

An audit of all 19 rules found **TM014 is the only vulnerable
rule**. The other "blank-line-around-*" rules (TM013, TM015)
and all line-based rules (TM001, TM006, TM007, TM008) are
safe -- the line-based rules already use
`lint.CollectCodeBlockLines()`, and pure AST-based rules are
naturally isolated since goldmark doesn't create child nodes
for code block content.

TM013 (blank-line-around-headings) uses the same pattern
(AST walk + raw line check) but is safe in practice because
goldmark never creates Heading nodes inside fenced code blocks.
However, it should get the same defensive fix for robustness.

## Root Cause

TM014 walks the AST for `*ast.List` nodes, then reads raw
`f.Lines[idx]` to check if adjacent lines are blank. There are
two possible scenarios that trigger the bug:

1. **Goldmark misparses indented fences** -- When a fenced
   code block is indented inside a list item, goldmark may
   fail to recognize it as a code fence. YAML list markers
   (`- item`) inside the "code block" then get parsed as
   regular List nodes, and TM014 operates on them.

2. **Multi-pass fixer interaction** -- TM016 (list-indent)
   may change indentation in an earlier pass, causing goldmark
   to misparse the code fence on re-parse in a later pass.

Either way, the fix is the same: TM014 must skip lists whose
lines overlap with code block regions.

## Approach

Use `lint.CollectCodeBlockLines()` (already exists, used by
TM001/TM006/TM007/TM008) to build a set of code block line
numbers, then skip any List node whose start or end line falls
within that set. Apply the same pattern to TM013 defensively.

If the reproduction test reveals that `CollectCodeBlockLines()`
itself misses the code block (because goldmark misparses the
fence), fall back to a raw line-based fence tracker that scans
for `` ``` `` / `~~~` patterns independently of the AST.

## Files to Modify

- `internal/rules/blanklinearoundlists/rule.go` -- add code
  block awareness to Check and Fix
- `internal/rules/blanklinearoundlists/rule_test.go` -- add
  reproduction test and regression tests
- `internal/rules/blanklinearoundheadings/rule.go` -- same
  defensive fix for Check and Fix
- `internal/rules/blanklinearoundheadings/rule_test.go` -- add
  matching tests
- `.tidymark.yml` -- re-enable `blank-line-around-lists: true`

## Existing Code to Reuse

- `internal/lint/codeblocks.go`:
  `CollectCodeBlockLines(f *File) map[int]bool` -- walks AST
  for FencedCodeBlock and CodeBlock nodes, returns set of
  1-based line numbers inside code blocks (including fences)

## Tasks

### A. Research goldmark upstream

0. Research whether goldmark has a known issue or ticket for
   misparsing fenced code blocks inside list items (indented
   fences). Check the goldmark GitHub repository for relevant
   issues and PRs. If the root cause is a goldmark parser bug,
   document findings and consider whether an upstream fix or
   workaround is more appropriate.

### B. Reproduce and fix

1. Write a reproduction test in
   `blanklinearoundlists/rule_test.go`: a markdown file with a
   fenced code block containing YAML list markers (`- item`)
   inside a numbered list item. Verify Check produces no
   diagnostics and Fix does not modify the content.

2. Run the test to confirm it fails (red).

3. In TM014's `Check` method: call
   `lint.CollectCodeBlockLines(f)` at the top, then inside the
   AST walk, skip any List node where `listStartLine` or
   `listEndLine` is in the code block set.

4. In TM014's `Fix` method: apply the same
   `CollectCodeBlockLines` filter.

5. Run the reproduction test to confirm it passes (green).

6. If step 2 shows the test passes unexpectedly (goldmark
   parses correctly and no List nodes exist inside the code
   block), investigate further: try the exact indentation
   pattern from the plan files, test multi-pass fix scenarios.

### C. Defensive fix for TM013

7. Apply the same defensive pattern to TM013: call
   `CollectCodeBlockLines(f)` in both Check and Fix, skip
   any Heading node whose line is in the code block set.

8. Add tests for TM013: code block containing `# heading`
   text should produce no TM013 diagnostics.

### D. Edge case tests

9. Add edge case tests for TM014:

  - List immediately before a fenced code block (valid
    diagnostic, should still fire)
  - List immediately after a fenced code block (valid
    diagnostic, should still fire)
  - List inside an indented code block (no diagnostic)
  - Empty fenced code block adjacent to a list

### E. Re-enable and verify

10. Re-enable `blank-line-around-lists: true` in
    `.tidymark.yml`.

11. Run `tidymark check plan/` to verify plan files pass
    (this was the original trigger).

12. Run full test suite: `go test ./...`

13. Run linter: `go tool golangci-lint run`

## Acceptance Criteria

- [ ] Goldmark upstream research documented (issue link or
      confirmation that no existing ticket covers this)
- [ ] `go test ./internal/rules/blanklinearoundlists/...`
      passes with reproduction and edge case tests
- [ ] `go test ./internal/rules/blanklinearoundheadings/...`
      passes with defensive tests
- [ ] TM014 does not report diagnostics for content inside
      fenced code blocks
- [ ] TM014 fixer does not insert blank lines inside fenced
      code blocks
- [ ] TM013 defensively skips code block interiors
- [ ] `blank-line-around-lists: true` re-enabled in
      `.tidymark.yml`
- [ ] `tidymark check plan/` passes with TM014 re-enabled
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
