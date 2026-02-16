---
id: 60
title: DU-Style Metrics Ranking
status: ✅
---
# DU-Style Metrics Ranking

## Goal

Add a `du`/`gdu`-style command for Markdown files.
It should show file-level metrics in a table.
Users can select columns and sort by one metric.
All metric consumers should use one shared abstraction.
Metrics should also have rules-like docs:
folder-based READMEs and `mdsmith help metrics`.
This supports workflows like "top 10 least concise files"
or "top 10 biggest files."

## Tasks

1. Create a generic metrics abstraction in `internal/metrics`.
   Define shared types for metric identity, scope, and numeric values.
2. Add a central registry in `internal/metrics`
   for metric lookup, defaults, and metadata.
3. Route metric computation through this abstraction.
   New metric logic should not live in command-specific code paths.
4. Migrate existing metric-producing code paths
   to call the shared abstraction where applicable.
5. Define a metrics docs layout similar to rules:
   `internal/metrics/<id>-<name>/README.md`
   with front matter fields for ID, name, and description.
6. Add embedded metrics docs lookup and listing,
   parallel to rule docs.
7. Extend `mdsmith help` with:
   `mdsmith help metrics` and
   `mdsmith help metrics <id|name>`.
8. Define CLI surface using:
   `mdsmith metrics list` and `mdsmith metrics rank`.
9. Implement `metrics list`
   (`--scope` and `--format`).
10. Implement `metrics rank` with flags:
    `--metrics`, `--by`, `--order`, `--top`, and `--format`.
11. Reuse existing file discovery behavior from `check`
    (paths, dirs, globs, `.gitignore`, and override handling).
12. Implement initial file metrics in the shared registry:
    `bytes`, `lines`, `words`, `headings`, `token-estimate`,
    and `conciseness` (when available from conciseness work).
13. Implement output rendering for both `text` and `json` formats.
    Text output should be table-oriented and easy to scan, similar to `du`.
14. Implement deterministic sorting and `--top N` limiting.
    Tie-break by path to keep output stable across runs.
15. Add docs and examples for common workflows:
    top 10 least concise files, top 10 largest files,
    and selected-column reports.
16. Add unit and e2e coverage for abstraction contracts,
    migration behavior, parsing, docs lookup, help output,
    sorting, limiting, formatting, and unknown metric errors.

## Acceptance Criteria

- [x] `mdsmith metrics rank --by conciseness --top 10 .`
      returns the 10 least concise Markdown files.
- [x] `mdsmith metrics rank --by bytes --top 10 .`
      returns the 10 largest Markdown files.
- [x] `mdsmith metrics rank --metrics bytes,lines,words --by bytes .`
      shows only selected metric columns and file path.
- [x] `mdsmith metrics list` shows available metrics
      from the shared registry.
- [x] `mdsmith help metrics` lists metrics with short descriptions.
- [x] `mdsmith help metrics <id|name>` prints that metric README.
- [x] `mdsmith metrics rank --format json` returns deterministic output
      sorted by the selected metric.
- [x] Metric definitions live in one shared abstraction
      and are not duplicated across command or rule code.
- [x] Existing metric consumers use the shared abstraction
      when computing metrics.
- [x] Metrics docs follow the folder + README structure
      and are embedded for offline help.
- [x] File discovery semantics match `mdsmith check`.
- [x] Unknown metrics return a clear actionable error.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
