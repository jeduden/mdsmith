---
id: 53
title: Conciseness Scoring
status: 🔲
---
# Conciseness Scoring

## Goal

Measure information density and flag verbose Markdown content
that remains readable but unnecessarily long.

## Tasks

1. Define conciseness heuristics
   (filler words, hedge phrases, low content-to-token ratios).
2. Implement scoring per paragraph and configurable thresholds.
3. Emit warnings with suggested targets and examples.
4. Document configuration and rationale.

## Acceptance Criteria

- [ ] Rule flags paragraphs that exceed a configurable verbosity threshold.
- [ ] Output includes the paragraph location and conciseness score.
- [ ] Heuristics are configurable and documented.
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
