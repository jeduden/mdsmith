---
id: 50
title: Redundancy / Duplication Detection
status: ✅
summary: >-
  MDS037 duplicated-content fingerprints paragraphs
  by SHA-256 over lowercase, whitespace-normalized
  text and scans the corpus (RootFS or the file's
  directory) for matches above a configurable
  min-chars threshold; opt-in by default.
---
# Redundancy / Duplication Detection

## Goal

Flag substantial duplicated content across Markdown files
to reduce token waste and drift.

## Tasks

1. Chose paragraph fingerprints (SHA-256 over a
   lowercase, whitespace-collapsed, trimmed form)
   with a `min-chars` threshold defaulting to 200
   runes.
2. Implemented cross-file comparison via
   [MDS037](../internal/rules/MDS037-duplicated-content/README.md);
   scope follows `RootFS` when the project root is
   known and falls back to the file's directory, and
   `include`/`exclude` globs scope the walk.
3. Diagnostics point to the self line and the
   other file's path and line
   (`paragraph duplicated in {other}:{line}`).
4. README documents the *O(N²)* read pattern and
   recommends an `exclude` entry for generated or
   vendored directories.

## Acceptance Criteria

- [x] Rule identifies duplicated sections beyond
      a configurable similarity threshold.
- [x] Findings include both source and duplicate locations.
- [x] Rule supports include/exclude patterns to limit scope.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
