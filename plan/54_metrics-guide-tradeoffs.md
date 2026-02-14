---
id: 54
title: Conciseness Metrics Design and Implementation
status: 🔲
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

- [ ] Conciseness metric is specified
      with documented heuristics and default thresholds.
- [ ] Rule is implemented with configurable settings and per-path overrides.
- [ ] Tests cover representative readable, technical, and verbose cases.
- [ ] Documentation explains how to tune conciseness thresholds
      and when to prefer other rules.
