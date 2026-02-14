---
id: 50
title: Redundancy / Duplication Detection
status: 🔲
---
# Redundancy / Duplication Detection

## Goal

Flag substantial duplicated content across Markdown files
to reduce token waste and drift.

## Tasks

1. Choose similarity strategy
   (e.g., shingled hashing or paragraph fingerprints) and thresholds.
2. Implement cross-file comparison with configurable scope and minimum size.
3. Emit findings that point to the duplicated segments and source files.
4. Document performance considerations and configuration.

## Acceptance Criteria

- [ ] Rule identifies duplicated sections beyond
      a configurable similarity threshold.
- [ ] Findings include both source and duplicate locations.
- [ ] Rule supports include/exclude patterns to limit scope.
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
