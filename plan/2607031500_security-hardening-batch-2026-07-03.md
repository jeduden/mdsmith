---
id: 2607031500
title: "Security hardening batch — 2026-07-03 post-audit diff review (low)"
status: "🔲"
summary: >-
  Two low-severity findings from the 2026-07-03 diff review: route
  Workspace.ReadFile through the same OpenRoot symlink containment as
  its sibling FS() view (F001), and cap block-quote recursion depth in
  the Layer-0 parse-skip scanner before either gating spike flag ships
  as a supported option (F002).
model: sonnet
---
# Security hardening batch — 2026-07-03 post-audit diff review (low)

## Goal

Address the two low-severity findings from the [2026-07-03 post-audit
diff review](../docs/security/2026-07-03-post-audit-diff-review/report.md).
Neither is exploitable today under default configuration, but each
closes a gap that would matter if the surrounding code evolves.

**F001 (low, CWE-61).** `OSWorkspace.ReadFile` in
`pkg/mdsmith/workspace.go:72-74` reads a joined path with no
containment check. `OverlayWorkspace.ReadFile` in
`pkg/mdsmith/overlay.go:57-66` has the same gap. A symlink escaping the
root is followed, not refused.

Their sibling `FS()` views got an `OpenRootFS` fix this window. This
direct seam did not. `Session.frontMatterFor` in
`pkg/mdsmith/session.go:431` calls the unguarded seam. It backs the
public `Session.Kinds` API.

**F002 (low, CWE-674).** `tryBlockquote` in
`internal/lint/layer0.go:224` recurses once per block-quote nesting
level. It has no depth cap. `lineHasNonFenceCode` in
`internal/lint/layer0_para.go:204-209` treats any nested `>` marker as
code-capable. That triggers the recursion.

A deeply nested quote line can exhaust the stack. Go cannot recover a
stack overflow. That would bypass this window's new panic-recovery
work. The path needs the `MDSMITH_LAYER0_SKIP` or
`MDSMITH_SPIKE_FLAT_L0` env var. Both are unwired and undocumented.
The gap is unreachable today, but worth a fix before either ships.

## Tasks

### F001 — contain Workspace.ReadFile through OpenRootFS

- [ ] Write a failing test for `OSWorkspace.ReadFile` on a
  within-workspace symlink pointing outside `Root`, mirroring the
  existing `FS()`-containment test in `pkg/mdsmith/workspace_test.go`
  (around line 358) — assert the read is refused, not followed.
- [ ] Write the matching failing test for `OverlayWorkspace.ReadFile`
  in `pkg/mdsmith/overlay_test.go` (mirroring lines 61-72's `FS()`
  containment test).
- [ ] Make `OSWorkspace.ReadFile` route through `w.FS()` (e.g.
  `fs.ReadFile(w.FS(), relPath)`) when `Root` is set, instead of calling
  `os.ReadFile` directly, so it shares `lint.OpenRootFS`'s containment.
  Keep the existing absolute-path passthrough behavior.
- [ ] Make `OverlayWorkspace.ReadFile`'s disk fall-through do the same,
  reusing `w.diskFS` (already built via `lint.OpenRootFS` in `FS()`)
  instead of a fresh `os.ReadFile` call.
- [ ] Remove the now-inapplicable `//nolint:gosec` comments on both
  methods.
- [ ] Run the two new tests to confirm both go green.

### F002 — cap block-quote recursion depth in the Layer-0 scanner

- [ ] Write a failing test driving `scanLayer0` with a line of enough
  nested `>` markers to exceed a proposed depth cap (e.g. 100), and
  assert it returns gracefully (e.g. treats the excess depth as plain
  text) rather than recursing further.
- [ ] Add a `maxBlockquoteDepth` constant near `maxIncludeDepth`'s
  existing pattern, and thread a depth counter through `tryBlockquote`'s
  recursion into `scanLayer0`, refusing to recurse past the cap.
- [ ] Confirm the depth-cap test passes and existing block-quote
  fixtures (nested code blocks, lazy continuations) are unaffected.

## Acceptance Criteria

- [ ] A within-workspace symlink escaping the workspace root is refused
  by `OSWorkspace.ReadFile` and `OverlayWorkspace.ReadFile`, not just
  their `FS()` views.
- [ ] `Session.Kinds` on a symlinked path outside the workspace root
  returns an error instead of reading the external file's front matter.
- [ ] A line with a depth-cap-exceeding number of nested `>` markers no
  longer recurses unbounded in `scanLayer0`, under `MDSMITH_LAYER0_SKIP=1`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports no issues
