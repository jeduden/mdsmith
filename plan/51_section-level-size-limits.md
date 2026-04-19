---
id: 51
title: Section-Level Size Limits
status: ✅
---
# Section-Level Size Limits

## Goal

Enforce size limits on individual Markdown sections
to prevent oversized headings from hiding in otherwise compliant files.

## Tasks

1. [x] Define section boundary detection rules for heading levels.
2. [x] Add configuration for per-heading or per-pattern limits.
3. [x] Implement rule to count section length by lines or tokens.
4. [x] Document configuration and examples.

## Acceptance Criteria

- [x] Rule enforces size limits per heading level or heading pattern.
- [x] Findings include the heading title and measured size.
- [x] Limits can be configured independently of file-level limits.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues

## Notes

Implemented as MDS036 `max-section-length` (named to mirror
`max-file-length`, since limits are measured in lines). A section
spans from its heading line up to (but not including) the next heading
line of any level, or end of file. Nested subsections are measured
independently of their parent, so the limit applies to direct content
under each heading. Lookup order for the applicable max: `per-heading`
regex (first match), `per-level`, `max`. Disabled by default; enable
by setting `max` (or any `per-level` / `per-heading` entry) to a
positive integer in `.mdsmith.yml`.
