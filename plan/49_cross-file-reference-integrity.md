---
id: 49
title: Cross-File Reference Integrity
status: ✅
template:
  allow-extra-sections: true
---
# Cross-File Reference Integrity

## Goal

Detect broken Markdown links to other files and headings
to prevent context loss across agent docs.

## Tasks

1. Identify supported link formats (relative paths, anchors)
   and parsing approach.
2. Implement resolver for file existence and heading anchors.
3. Add rule configuration for include/exclude patterns and optional strictness.
4. Document usage and limitations.

## Acceptance Criteria

- [x] Rule reports missing target files for Markdown links.
- [x] Rule reports missing headings for anchor links.
- [x] Output includes the source file and the broken link target.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
