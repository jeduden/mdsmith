# Paragraph Readability Score

## Goal

Implement TM023 `paragraph-readability` that computes a
readability score for each paragraph and warns when it
exceeds a configurable threshold, encouraging clearer
writing. Uses the Automated Readability Index (ARI) by
default; the internal design supports adding alternative
formulas (Flesch-Kincaid, Coleman-Liau) later.

## Tasks

### A. Shared plain text extraction

1. Create `internal/mdtext/` package with
   `ExtractPlainText(node ast.Node, source []byte) string`
   that strips markdown syntax:

  - Inline code: keep text content
  - Links: keep display text, drop URL
  - Emphasis/strong: keep inner text
  - Images: keep alt text

   This helper is shared with TM024
   (`paragraph-structure`).

2. Add `CountWords(text string) int` and
   `CountSentences(text string) int` to the same package.
   `CountSentences` splits on `.`, `!`, `?` followed by
   whitespace or end of text. Treat zero sentences as 1.

### B. Readability computation

3. Create `internal/rules/paragraphreadability/readability.go`
   with a `GradeFunc` type and an ARI implementation:

   ```go
   // GradeFunc computes a readability grade level from
   // plain text. Higher values mean harder to read.
   type GradeFunc func(text string) float64
   ```

   ARI formula:

   ```text
   4.71 * (characters / words)
     + 0.5 * (words / sentences)
     - 21.43
   ```

   Where `characters` counts letters and digits only
   (no spaces or punctuation).

4. Handle edge cases: single-word paragraphs, no
   sentences (treat as 1), empty text (return 0).

### C. Rule implementation

5. Create `internal/rules/paragraphreadability/rule.go`
   implementing `rule.Rule` and `rule.Configurable`.
   Settings:

   | Setting | Type | Default | Description |
   |---------|------|---------|-------------|
   | `max-grade` | float | 14.0 | Max readability grade level |
   | `min-words` | int | 20 | Minimum words to trigger check |

   Paragraphs with fewer than `min-words` words are
   skipped (short paragraphs produce unreliable scores).

6. Check logic: walk the AST for `*ast.Paragraph` nodes.
   For each, extract plain text via `mdtext.ExtractPlainText`,
   count words, skip if below `min-words`, compute grade.
   If it exceeds `max-grade`, emit:

  - `paragraph readability grade too high (16.2 > 14.0)`

   Diagnostic on the paragraph's first line.

7. The rule struct holds a `GradeFunc` field, defaulting
   to ARI. This allows swapping to a different formula
   later without changing the rule logic.

8. Register rule, add blank import, set category to
   `"meta"`.

### D. Documentation

9. Write `rules/TM023-paragraph-readability/README.md`.

10. Create test fixtures with high-grade and low-grade
    paragraphs.

### E. Tests

11. Unit tests for `mdtext.ExtractPlainText`: links,
    emphasis, code spans, images, nested markup.

12. Unit tests for ARI: known reference texts with
    expected grade levels (within +-0.5 tolerance).

13. Unit tests for rule Check: paragraph over threshold,
    paragraph under threshold, short paragraph skipped,
    paragraph with inline markup.

14. Run `go test ./...` and `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] ARI grade computed for each paragraph
- [ ] Paragraphs below `min-words` skipped
- [ ] `max-grade` configurable, default 14.0
- [ ] Inline markup stripped before scoring
- [ ] `internal/mdtext/` shared package created
- [ ] `GradeFunc` abstraction supports future formulas
- [ ] Rule README with examples
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
