---
id: 2606211910
title: >-
  arch-fix: add trivial-accessor exemption
  comments in workspace.go
status: '✅'
summary: >-
  Add one-line "no test by design" comments
  to the trivial one-liner methods in
  pkg/mdsmith/workspace.go so the audit can
  distinguish intentional exemptions from
  forgotten tests.
model: ''
depends-on: []
---
# arch-fix: workspace.go trivial-accessor exemptions

## Context

Audit 2026-06-21 flagged trivial one-liner
methods in
[`pkg/mdsmith/workspace.go`](../pkg/mdsmith/workspace.go).

Tests doc §"Exemptions" says:

> Add a one-line comment on the function so
> the audit can distinguish "no test by
> design" from "no test, forgotten".

The affected methods are:

- `memFile.Close`
- `memDir.Read`
- `memDir.Close`
- `memDirEntry.Name`, `.IsDir`, `.Type`,
  `.Info`
- `memFileInfo.Name`, `.Size`, and the
  remaining one-liner interface methods

## Goal

Add a one-line exemption comment to each
affected trivial method.
Future audits pass without flagging them.

## Tasks

1. For each trivial one-liner method in
   `pkg/mdsmith/workspace.go`, add a comment:
   `// no test by design — trivial accessor`
2. Run `go build ./...` — confirm no errors.
3. Run `go run ./cmd/mdsmith check
   pkg/mdsmith/workspace.go` — confirm no
   new lint violations.

## Acceptance Criteria

- [x] Every trivial one-liner method in
  `pkg/mdsmith/workspace.go` carries an
  exemption comment.
- [x] `go build ./...` passes.
- [x] `mdsmith check pkg/mdsmith/workspace.go`
  reports no new violations.
