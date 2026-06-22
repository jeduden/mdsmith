---
id: 2606211908
title: 'arch-fix: split internal/lint/layer0.go'
status: "✅"
summary: >-
  Split layer0.go (1 203 lines) into focused
  sibling files along block-type sub-parsers
  to stay within the 1 000-line guideline.
model: ''
depends-on: []
---
# arch-fix: split internal/lint/layer0.go

## Context

Audit 2026-06-21 flagged
[`internal/lint/layer0.go`](../internal/lint/layer0.go)
at 1 203 lines.

The file holds the entire Layer-0 block
scanner. It packs seven sub-parsers together:

- Exported types (`BlockKind`, `BlockSpan`,
  `Layer0Scan`)
- Scanner state machine
- HTML-block detection (types 1–7)
- Fence handling
- ATX-heading detection
- Indented-code detection
- Paragraph scanning

One file for all of these makes targeted
changes risky. Locating the relevant section
is slow.

## Goal

Split `layer0.go` into sibling files within
`internal/lint`. Each sibling covers one or
two block-type sub-parsers. The primary file
stays under 600 lines.

## Tasks

1. Create `internal/lint/layer0_html.go`:
   move HTML-block detection (types 1–7),
   `openHTMLBlock`, `htmlBlockCloses`,
   `tagName`, and related helpers.
2. Create `internal/lint/layer0_fence.go`:
   move fence-state logic (`fenceInfo`,
   `openingFence`, `closingFence`,
   `advanceFenceState`, `tryFence`).
3. Create `internal/lint/layer0_para.go`:
   move paragraph scanning (`scanParagraph`,
   `markSetextRun`, `paragraphLeadKind`,
   `SourceMayHaveCodeBlock`).
4. Remove the moved blocks from `layer0.go`
   and trim now-unused file-local symbols.
5. Run `go build ./...` — confirm no errors.
6. Run `go test ./...` — confirm no
   regressions.
7. Confirm `wc -l layer0.go` is under
   600 lines.

## Acceptance Criteria

- [x] `internal/lint/layer0.go` is under
  600 lines.
- [x] `go build ./...` passes.
- [x] `go test ./...` passes.
- [x] `go tool golangci-lint run` reports
  no new issues.
- [x] No logic changed — pure file
  reorganisation within the `lint` package.
