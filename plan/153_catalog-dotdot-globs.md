---
id: 153
title: Catalog directive — accept `..` globs within project root
status: "🔲"
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

1. Add a failing test to the catalog
   [`rule_test.go`](../internal/rules/catalog/rule_test.go)
   for a `../sibling/*.md` glob whose resolved
   path stays inside the project root.
2. Add a failing test for a glob whose resolve
   escapes the project root and expects the new
   diagnostic message.
3. Replace the `containsDotDot` reject with a
   project-root containment check, sharing the
   resolve helper with MDS021.
4. Update the
   [MDS019 catalog README](../internal/rules/MDS019-catalog/README.md)
   to describe the new behavior and the
   escapes-root diagnostic; align wording with
   the MDS021 README.
5. Add a `good/` fixture exercising the new
   accept case and a `bad/` fixture exercising
   the new reject case under
   [`internal/rules/MDS019-catalog/`](../internal/rules/MDS019-catalog/).
6. Once the new behavior lands, replace the
   hand-maintained reference-style link defs in
   the
   [solid-architecture SKILL.md](../.claude/skills/solid-architecture/SKILL.md)
   with a `<?catalog?>` block targeting
   `../../../docs/development/architecture/*.md`
   using the `{slug}` front-matter field
   already present on each canonical doc, then
   run `mdsmith fix .` to regenerate.

## Acceptance Criteria

- [ ] Tests in
      `internal/rules/catalog/rule_test.go`
      cover the accept and reject cases and
      pass.
- [ ] `internal/rules/MDS019-catalog/README.md`
      documents the new behavior and lists the
      escapes-root diagnostic.
- [ ] `internal/rules/MDS019-catalog/good/`
      and `bad/` each contain a new fixture for
      the new behavior.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.
- [ ] `.claude/skills/solid-architecture/SKILL.md`
      uses a `<?catalog?>` block for the
      architecture link defs and renders the
      same five reference-style labels as
      before.
