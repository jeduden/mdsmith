---
id: 35
title: Paragraph Structure Limits
status: 🔲
---
# Paragraph Structure Limits

## Goal

Implement TM024 `paragraph-structure` that enforces
structural limits on markdown paragraphs: maximum sentences
per paragraph and maximum words per sentence, encouraging
concise writing. Reuses `internal/mdtext/` from Plan 34.

## Tasks

### A. Sentence splitting dependency

1. Run `go get github.com/neurosnap/sentences` to add the
   Punkt sentence tokenizer. It has zero transitive
   dependencies and handles abbreviations, decimals, and
   ellipses via a trained language model.

2. Add `SplitSentences(text string) []string` to
   `internal/mdtext/` as a thin wrapper around
   `neurosnap/sentences`:

  - Load the English training data once (package-level
     `sync.Once`)
  - Tokenize the input text
  - Return the text of each sentence, filtering empty
     entries

### B. Rule implementation

3. Create `internal/rules/paragraphstructure/rule.go`
   implementing `rule.Rule` and `rule.Configurable`.
   Settings:

   | Setting | Type | Default | Description |
   |---------|------|---------|-------------|
   | `max-sentences` | int | 6 | Max sentences per paragraph |
   | `max-words` | int | 40 | Max words per sentence |

4. Check logic: walk AST for `*ast.Paragraph` nodes.
   For each paragraph:

  - Extract plain text via `mdtext.ExtractPlainText`
  - Split into sentences via `mdtext.SplitSentences`
  - If sentence count exceeds `max-sentences`:
     `paragraph has too many sentences (8 > 6)`
  - For each sentence exceeding `max-words` (using
     `mdtext.CountWords`):
     `sentence too long (45 > 40 words)`
  - Diagnostics on the paragraph's first line

5. Register rule, add blank import, set category to
   `"meta"`.

### C. Documentation

6. Write `rules/TM024-paragraph-structure/README.md`.

7. Create test fixtures:

  - `bad.md`: paragraph with 8 sentences, sentence
     with 50 words
  - `good.md`: well-structured paragraphs

### D. Tests

8. Unit tests for `mdtext.SplitSentences`:

  - Simple sentences with `.`, `!`, `?`
  - Abbreviations not split (`e.g.`, `Dr.`)
  - Ellipsis handled
  - Decimal numbers not split
  - Empty input

9. Unit tests for rule Check:

  - Paragraph over `max-sentences`: diagnostic
  - Paragraph under limit: no diagnostic
  - Sentence over `max-words`: diagnostic
  - Short paragraph: no diagnostic
  - Custom settings respected
  - Both limits exceeded: two diagnostics

10. Run `go test ./...` and `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `neurosnap/sentences` added as dependency
- [ ] `mdtext.SplitSentences` wraps Punkt tokenizer
- [ ] `max-sentences` enforced per paragraph, default 6
- [ ] `max-words` enforced per sentence, default 40
- [ ] Uses `mdtext.ExtractPlainText` and `mdtext.CountWords`
      from Plan 34
- [ ] Diagnostics include counts
- [ ] Rule README with examples
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
