---
id: 51
title: Section-Level Size Limits
status: 🔲
---
# Section-Level Size Limits

## Goal

Enforce size limits on individual Markdown sections
to prevent oversized headings from hiding in otherwise compliant files.

## Tasks

1. Define section boundary detection rules for heading levels.
2. Add configuration for per-heading or per-pattern limits.
3. Implement rule to count section length by lines or tokens.
4. Document configuration and examples.

## Acceptance Criteria

- [ ] Rule enforces size limits per heading level or heading pattern.
- [ ] Findings include the heading title and measured size.
- [ ] Limits can be configured independently of file-level limits.
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
