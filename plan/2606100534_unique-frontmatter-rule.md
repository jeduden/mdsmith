---
id: 2606100534
title: 'Workspace-unique front-matter fields (unique-frontmatter rule)'
status: "✅"
model: sonnet
summary: >-
  New rule MDS069: within configured include/exclude globs,
  no two files may carry the same value in a named
  front-matter field. Enabled for plan ids so a same-minute
  id collision fails mdsmith check on the second PR's
  merge-queue run.
depends-on: [2606100533]
---
# Workspace-unique front-matter fields (unique-frontmatter rule)

## Problem

Timestamp ids
([2606100533](2606100533_timestamp-plan-ids.md)) shrink
the plan-id race to one UTC minute but cannot remove it.
Nothing fails today when two plan files share an id. The
nine legacy pairs all merged green. The merge queue tests
the merged result. A workspace uniqueness check would
therefore fail the second PR as soon as both files meet.
At that moment the fix is a one-commit rename.

## Goal

A rule that reports a repeated front-matter value within
a configured file scope. The later file in path order
gets the diagnostic. Its message names the field, the
value, and the earlier file. An id collision then
surfaces as a `mdsmith check` failure, not as a silent
duplicate.

## Design

### Rule and settings

New rule `unique-frontmatter`, MDS069, under
`internal/rules/uniquefrontmatter/`. Settings are flat,
mirroring cross-file-reference-integrity's idiom:

- `field:` — the front-matter key whose values must be
  distinct. Files without the field are skipped.
- `include:` / `exclude:` — glob lists selecting the
  scope, with [globs.md](../docs/reference/globs.md)
  semantics. Empty `include` disables the rule, so it
  ships enabled by default and inert.

A list-of-scopes shape stays out until a second use case
exists; every comparable rule grew from flat settings.
The list settings replace on merge (the default); the
choice is documented next to `ApplySettings`.

The include globs intentionally repeat the plan kind's
glob from kind-assignment. Rule settings have no
precedent for referencing kinds, so the two must be
updated together if plan files ever move.

### Mechanics

Diagnostics are host-file-anchored, like MDS027: when
`Check(f)` sees that an earlier in-scope file already
holds f's value, it reports on f at its front-matter
line. The earlier file stays clean. One diagnostic per
later file. No `rule.RepoScoped` marker — each file's
diagnostic tuple differs, so DedupeDiagnostics is not
involved.

Example: with `plan/a.md` and `plan/b.md` both carrying
`id: 7`, only `plan/b.md` is flagged:

```text
front-matter "id": value 7 already used by plan/a.md
```

The value-to-first-file index builds once per run, in a
dedicated `RunCache` slot (`UniqueFieldIndex`). The
index registers a scope matcher with the slot, so an
LSP edit drops it only when the edited path falls inside
the scope's globs — an unrelated edit keeps the index
warm. Hosts without a `RunCache` fall back to the
per-File memo. The index reads the workspace as saved on
disk, the same view every cross-file rule gets; routing
the LSP's unsaved-buffer overlay into `RootFS`
engine-wide is follow-up work.

### Allocation budget

The repo-wide budget gate's fixture configures no
scopes, so it only exercises the inert path. The rule
therefore ships its own alloc test. That test configures
settings, warms the cache, and asserts ≤ 10 allocs per
Check on the steady state — the repo's per-package
alloc-test pattern, applied to `Check` itself rather
than a helper.

### Enabling for plan ids

Enablement is gated on the release train. The
`mdsmith-fixed-version` CI job runs the pinned release
binary. That binary predates MDS069 and silently ignores
the unknown rule key. Early config would therefore
enforce uniqueness under the branch binary but not the
pinned one. The block lands after the rule ships in a
tagged release and the pin bumps. That keeps both jobs
consistent — the
[adopt-new-syntax process](../docs/development/adopt-new-directive-syntax.md).

This plan stays 🔳 with task 4 open as the tracked
reminder. The [.mdsmith.yml](../.mdsmith.yml) edit needs
maintainer consent. Approving this plan grants it for
exactly the block below. It nests under the top-level
`rules:` key:

```yaml
rules:
  unique-frontmatter:
    field: id
    include: ["plan/*.md"]
    exclude: ["plan/proto.md"]
```

A rule, not workflow shell. The
[release-tooling page](../docs/development/release-tooling.md)
bars inline logic in workflows. Per-scope unique fields
are also a reusable cross-file integrity feature worth
dogfooding: unique ids or titles across any kind.

## Tasks

1. [x] Red/green unit tests in `rule_test.go`, then the
   rule: registration, settings parsing, the run-scoped
   index, and the diagnostic on the later file.
2. [x] The configured-path alloc test described above;
   keep `Check` within ≤ 10 allocs/op on the steady
   state.
3. [x] Fixtures under
   `internal/rules/MDS069-unique-frontmatter/` (good/bad
   with expected diagnostics) plus the rule README on the
   rule-readme schema.
4. [x] Gated on the pin bump: after a release ships
   MDS069 and `MDSMITH_VERSION` in
   `setup-mdsmith-pinned-version` moves to it, add the
   plan-id block to [.mdsmith.yml](../.mdsmith.yml)
   (consent above) and verify a duplicate plan id fails
   `mdsmith check .` while the clean tree passes. Done
   with the v0.40.0 pin bump, which ships MDS069.

## Acceptance Criteria

- [x] (Gated on task 4.) A duplicated plan id added
  locally makes `mdsmith check .` fail; the diagnostic
  lands on the later file in path order and its message
  names the field, the value, and the earlier file.
  Verified once pre-gate with a temporary config block
  and a probe file: the probe failed at its `id:` line
  naming the first holder. Re-verified with the live
  block: the probe failed the same way, and the clean
  tree passes under both the branch binary and the
  pinned v0.40.0 one.
- [x] The bad fixture yields exactly one diagnostic per
  extra file sharing a value; the good fixture is clean.
- [x] A file missing the configured field, or outside the
  include/exclude scope, is never flagged.
- [x] The configured-path alloc test passes at ≤ 10
  allocs/op; the repo-wide budget gate stays green.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
