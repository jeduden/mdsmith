---
id: 173
title: Website rewriter tolerates titled links
status: "✅"
model: sonnet
depends-on: [170]
summary: >-
  Widen the website rewriter regexes to capture an
  optional link-title tail so a titled repo-relative
  link is rewritten and its title preserved instead of
  shipping as a dead path.
---
# Website rewriter tolerates titled links

## Goal

Stop `[x](../y.md "title")` from shipping as a dead
repo-relative path on the published site, closing gap
G6 from the
[link handling audit](../docs/research/links/README.md).

## Background

The audit found every rewrite regex in
[internal/release/website.go](../internal/release/website.go)
stops the path capture at whitespace (`\S+`), so a
Markdown title makes the link fail to match and pass
through unrewritten. The corpus has 0 titled links in
`docs/` today, so this is low-priority and pre-emptive,
but the gap is real and cheap to close.

## Tasks

1. Add a red test: a synced doc containing
   `[x](../../../docs/a.md "t")` currently keeps the
   repo-relative path.
2. Widen the affected regexes to capture an optional
   `(\s+"[^"]*")?` title tail and re-emit it after the
   rewritten path. Keep the `applyOutsideCode` guard.
3. Verify reference-def forms (`[label]: path "t"`) are
   handled the same way.
4. Green the test; confirm no existing rewrite
   regressed.

## Acceptance Criteria

- [x] A titled inline link is rewritten with its title
  preserved.
- [x] A titled reference definition is rewritten with
  its title preserved.
- [x] Code spans and fences are still untouched.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no issues.
