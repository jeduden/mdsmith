# Fix TM014 Fixer Corrupting Code Blocks

## Goal

Fix a bug where the TM014 (blank-line-around-lists) fixer
inserts blank lines inside fenced code blocks, corrupting file
content. The check and fix logic must skip content inside
fenced code blocks.

## Bug Description

When a fenced code block contains YAML list markers (`- item`)
or thematic break markers (`---`), TM014 incorrectly treats
them as markdown list items and inserts blank lines around
them during fix. This corrupts code blocks by adding spurious
blank lines between the content and the closing fence.

Reproduction: run `tidymark fix` on a markdown file containing
a fenced code block with YAML list content indented inside a
numbered list item. The fixer inserts blank lines inside the
code block.

## Tasks

### A. Investigate

1. Verify the bug in TM014's `Check` method: confirm that
   it reports diagnostics for list-like content inside
   fenced code blocks.

2. Verify the bug in TM014's `Fix` method: confirm that
   it inserts blank lines inside fenced code blocks.

3. Check whether TM013 (blank-line-around-headings) and
   TM015 (blank-line-around-fenced-code) have the same
   class of bug (operating inside code blocks).

### B. Fix

4. Update the TM014 check and fix logic to track whether
   the current line is inside a fenced code block. Skip
   all list detection and blank-line insertion for lines
   inside fenced code blocks.

5. If TM013 or TM015 have the same bug, apply the same
   fix pattern.

6. Fenced code block detection: track open/close fence
   markers (`` ``` `` or `~~~`) with matching indent
   levels. Handle indented fences (0-3 spaces) and
   fences inside list item continuations.

### C. Re-enable TM014

7. Change `blank-line-around-lists: false` back to `true`
   in `.tidymark.yml`.

8. Run `tidymark check` on the full project to verify
   no false positives remain.

### D. Tests

9. Add unit test: code block containing YAML list markers
   (`- item`) produces no TM014 diagnostics.

10. Add unit test: code block containing `---` produces
    no TM014 diagnostics.

11. Add unit test: fixer does not modify content inside
    fenced code blocks.

12. Add unit test: list immediately before or after a
    fenced code block still gets correct blank-line
    diagnostics (ensure the fix does not suppress valid
    diagnostics outside code blocks).

13. Add regression test with the exact input that
    triggered the bug (YAML code block inside a numbered
    list item, indented 3 spaces).

## Acceptance Criteria

- [ ] TM014 does not report diagnostics for content
      inside fenced code blocks
- [ ] TM014 fixer does not insert blank lines inside
      fenced code blocks
- [ ] TM013 and TM015 also skip code block interiors
      (if affected)
- [ ] `blank-line-around-lists: true` re-enabled in
      `.tidymark.yml`
- [ ] `tidymark check` passes on the full project
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
