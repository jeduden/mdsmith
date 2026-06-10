---
id: 2606022123
title: Catalog directive — accept `..` globs within project root
status: "✅"
summary: >-
  Let <?catalog?> accept `..` segments in glob
  patterns as long as the resolved pattern stays
  within the project root, matching <?include?>'s
  existing behavior. Unblocks cross-tree catalogs
  (e.g. a skill cataloging the architecture docs
  one level up the tree).
model: sonnet
depends-on: []
---
# Catalog directive — accept `..` globs within project root

## Goal

Bring `<?catalog?>` glob acceptance in line with
`<?include?>`: `..` segments are allowed as long
as the resolved pattern stays inside the project
root, unblocking cross-tree catalogs.

## Background

[`internal/rules/catalog/rule.go`](../internal/rules/catalog/rule.go)
rejects any glob containing a `..` segment
outright. The companion `<?include?>` directive
(MDS021) accepts `..` paths and only fails when
the resolved path escapes the project root (see
[MDS021's README](../internal/rules/MDS021-include/README.md)).

The asymmetry shows up in real use. The
[solid-architecture skill](../.claude/skills/solid-architecture/SKILL.md)
wants to catalog the architecture docs at
[the architecture hub](../docs/development/architecture/index.md)
and its siblings. The skill sits four levels
deep, so the glob would need a
`../../../docs/...` prefix, which today is
rejected. The skill ships a hand-maintained
reference-style link-def block as a workaround;
relaxing this rule retires that workaround.

## Non-goals

- Allowing absolute glob paths. Project-root
  containment is the rule; absolute paths still
  reject.
- Allowing globs that resolve outside the
  project root, even via `..`.
- Loosening the `<?include?>` directive — it
  already does the right thing.

## Design

Refactor `validateGlob` in
[`internal/rules/catalog/rule.go`](../internal/rules/catalog/rule.go):

1. Stop rejecting patterns wholesale on the
   first `..` segment.
2. After the existing absolute-path check,
   resolve the pattern against the marker
   file's directory and compare to the project
   root.
3. Accept the pattern if it stays inside the
   project root.
4. Reject with a new diagnostic
   `generated section directive glob escapes
   project root` when the pattern would
   resolve outside the project root.

A resolve helper already exists for MDS021.
Either share it from the
[include rule](../internal/rules/include/) or
lift it into [globpath](../internal/globpath/).
Have both directives call the shared helper.

## Tasks

1. [x] Add a test for a `../sibling/*.md` glob
   whose resolved path stays inside the project
   root (`TestCatalog_DotDotGlobStaysInsideRoot`).
2. [x] Add a test for a glob whose resolve
   escapes the project root and expects the new
   diagnostic message
   (`TestCatalog_DotDotGlobEscapesRoot`).
3. [x] Replace the `containsDotDot` reject with a
   project-root containment check; the shared
   resolve helper now lives in
   `internal/globpath` as `ResolveAgainstRoot`
   (alongside `ContainsDotDotSegment`). Catalog
   defers there; the MDS021 helper can adopt it
   in a follow-up since its current path-escape
   check predates the helper.
4. [x] Update the
   [MDS019 catalog README](../internal/rules/MDS019-catalog/README.md)
   to describe the new behavior and the
   escapes-root and missing-root diagnostics.
5. [x] Add `good/dotdot.md` and `bad/dotdot.md`
   under
   [`internal/rules/MDS019-catalog/`](../internal/rules/MDS019-catalog/).
   The integration runner now pins
   `f.RootFS = f.FS` for MDS019 fixtures so
   ".." resolution mirrors a real project.
6. [x] Add the `<?catalog?>` block in the
   [solid-architecture SKILL.md](../.claude/skills/solid-architecture/SKILL.md)
   targeting
   `../../../docs/development/architecture/*.md`
   with `row: "[{slug}]: {filename}"`. Running
   `mdsmith fix` regenerates the five
   slug-labeled link defs.

## Acceptance Criteria

- [x] Tests in
      `internal/rules/catalog/rule_test.go`
      cover the accept and reject cases and
      pass.
- [x] `internal/rules/MDS019-catalog/README.md`
      documents the new behavior and lists the
      escapes-root diagnostic.
- [x] `internal/rules/MDS019-catalog/good/`
      and `bad/` each contain a new fixture for
      the new behavior.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.
- [x] `.claude/skills/solid-architecture/SKILL.md`
      uses a `<?catalog?>` block for the
      architecture link defs and renders the
      five reference-style slug labels
      (`audit`, `cross`, `go`, `hub`, `ts`).
