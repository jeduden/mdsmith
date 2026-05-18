---
id: 171
title: MDS027 link-integrity hardening
status: "✅"
model: opus
depends-on: [170]
summary: >-
  Extend MDS027 to validate image targets, resolve
  absolute paths against a configured site root, and
  cover reference-style links, gated by a shared
  `links:` config block. Add a subpath-baseURL
  regression test for the Hugo render-link hook.
---
# MDS027 link-integrity hardening

## Goal

MDS027 passes three link forms silently. Close those
blind spots. They are gaps G1–G3 in the
[link handling audit](../docs/research/links/README.md).
Also lock in the G4 baseURL fix with a regression test.

## Background

The audit lives at
[docs/research/links/README.md](../docs/research/links/README.md).
It found that
[`linkgraph.ExtractLinks`](../internal/linkgraph/linkgraph.go)
walks only `*ast.Link` with `Reference == nil`. So
[MDS027](../internal/rules/crossfilereferenceintegrity/rule.go)
never resolves image targets (G1). It also
short-circuits absolute paths (G2).

It ignores reference-style links even though
[`ExtractRefLinks`](../internal/linkgraph/refs.go)
already exists (G3). The audit decided to extend
MDS027 rather than split it. The new behavior is gated
by a shared `links:` block.

## Tasks

1. Add a `links:` config block parser: `site-root`,
   `validate-images`, `validate-reference-style`.
   Deep-merge per kind like every other rule config.
2. G1: walk `*ast.Image` in the MDS027 resolver behind
   `validate-images` (default on). Red test: a missing
   `![](x.png)` reports a broken target.
3. G3: feed `linkgraph.ExtractRefLinks` through the
   same resolver behind `validate-reference-style`.
   Red test: a `[a]` with a broken `[a]:` def is
   flagged.
4. G2: when `site-root` is set, resolve absolute
   targets to a workspace path instead of
   short-circuiting. Red test: `/docs/rules/MDS027/`
   resolves; a missing one is flagged; unset preserves
   today's short-circuit.
5. G4 regression: assert the website render-link hook
   prefixes site-absolute links with
   `site.Home.RelPermalink` under a non-root baseURL.
6. Update MDS027's README and the audit doc's status if
   any decision changed during implementation.

## Acceptance Criteria

- [x] A broken `![](x.png)` is flagged by MDS027 with
  `validate-images` on and silent when off.
- [x] A broken reference-style target is flagged with
  `validate-reference-style` on.
- [x] An absolute target resolves against `site-root`
  when set and short-circuits when unset.
- [x] A subpath-baseURL regression test asserts the
  render-link prefix.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no issues.
