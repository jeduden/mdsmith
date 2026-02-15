---
id: 54
title: Conciseness Metrics Design and Implementation
status: ✅
template:
  allow-extra-sections: true
---
# Conciseness Metrics Design and Implementation

## Goal

Design and implement conciseness scoring metrics
(heuristics, thresholds, and configuration) for mdsmith,
backed by tests and documentation.

## Tasks

1. Specify candidate conciseness metrics and choose a baseline heuristic
   (filler/hedge ratios, content-to-token ratio, verbose phrase penalties).
2. Define tokenization and paragraph boundaries
   that align with existing mdtext utilities.
3. Calibrate default thresholds using a representative doc set
   and record false-positive risk.
4. Implement the conciseness rule with configurable thresholds,
   word/phrase lists, and per-path overrides.
5. Add unit tests and fixtures covering false positives,
   technical prose, and verbose-but-readable content.
6. Update rule docs and usage examples to explain configuration and trade-offs.

## Acceptance Criteria

- [x] Conciseness metric is specified
      with documented heuristics and default thresholds.
- [x] Rule is implemented with configurable settings and per-path overrides.
- [x] Tests cover representative readable, technical, and verbose cases.
- [x] Documentation explains how to tune conciseness thresholds
      and when to prefer other rules.

## Implementation Notes

- Added rule: `MDS026` (`conciseness`) in `internal/rules/conciseness/`.
- Heuristic combines:
  - filler ratio penalty,
  - hedge ratio penalty,
  - verbose phrase density penalty,
  - low content-ratio penalty.
- Paragraph boundaries and text extraction follow existing mdtext utilities:
  `mdtext.ExtractPlainText` + paragraph AST traversal.
- Default settings:
  - `min-score: 55.0`
  - `min-words: 20`
  - `min-content-ratio: 0.45`
  - `filler-weight: 1.0`
  - `hedge-weight: 0.8`
  - `verbose-phrase-weight: 4.0`
  - `content-weight: 1.2`
- Configurable lexical lists:
  - `filler-words`
  - `hedge-words`
  - `verbose-phrases`
- Added unit tests and rule fixtures for:
  - technical prose that should pass,
  - verbose prose that should fail,
  - verbose-but-readable/tuning behavior,
  - settings validation and table skipping.
