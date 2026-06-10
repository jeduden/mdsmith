---
id: 2606100534
title: 'Workspace-unique front-matter fields (unique-frontmatter rule)'
status: "🔲"
model: sonnet
summary: >-
  Repo-scoped rule that reports two files in a configured
  glob scope carrying the same value in a named
  front-matter field, enabled for plan ids so a same-minute
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
a configured scope. The later file in path order gets the
diagnostic, naming the field, the value, and the first
file. An id collision then surfaces as a `mdsmith check`
failure, not as a silent duplicate.

## Design

- New rule `unique-frontmatter` under
  `internal/rules/uniquefrontmatter/`, taking the next
  free MDS number (MDS069 at the time of writing).
- Setting `scopes:` is a list of `{glob, field}` entries.
  `glob` is a glob list with `!` exclusions per
  [globs.md](../docs/reference/globs.md); `field` names a
  front-matter key. Within one scope, every file matching
  the globs and carrying the field must hold a distinct
  scalar value. Files without the field are skipped.
- Repo-scoped, like cross-file-reference-integrity: mark
  it `rule.RepoScoped` so DedupeDiagnostics keeps the
  per-file diagnostics (see
  [plan 183](183_dedupe-diagnostics-repo-scoped-skip.md)).
- The diagnostic lands on the later file in path order,
  at its front-matter line:

  ```text
  front-matter "id": value 2606100533 already used by
  plan/2606100533_timestamp-plan-ids.md
  ```

- `scopes` replaces on merge (the default for list
  settings); document that choice next to its
  `ApplySettings` handler.
- With no `scopes` configured the rule reports nothing,
  so it ships enabled by default and inert.
- This repo enables it for plan ids. The
  [.mdsmith.yml](../.mdsmith.yml) edit needs maintainer
  consent; approving this plan grants it for exactly this
  block:

  ```yaml
  unique-frontmatter:
    scopes:
      - glob: ["plan/*.md", "!plan/proto.md"]
        field: id
  ```

- A rule, not workflow shell. The
  [release-tooling page](../docs/development/release-tooling.md)
  bars inline logic in workflows. Per-scope unique fields
  are also a reusable cross-file integrity feature worth
  dogfooding: unique ids or titles across any kind.

## Tasks

1. [ ] Red/green unit tests in `rule_test.go`: a
   duplicated scalar across two files diagnoses the later
   path and names the first; a missing field skips; files
   outside the globs skip; distinct values pass.
2. [ ] Implement `Check` within the ≤ 10 allocs/op budget
   (the alloc budget test must cover the new rule); reuse
   existing front-matter parsing helpers rather than
   adding a second YAML parse per file.
3. [ ] Fixtures under
   `internal/rules/MDS069-unique-frontmatter/` (good/bad
   with expected diagnostics) plus the rule README on the
   rule-readme schema.
4. [ ] Document the `scopes` replace-on-merge choice next
   to the `ApplySettings` handler.
5. [ ] With consent, add the plan-id scope to
   [.mdsmith.yml](../.mdsmith.yml) and verify
   `mdsmith check .` passes on the renumbered tree.

## Acceptance Criteria

- [ ] A duplicated plan id added locally makes
  `mdsmith check .` fail, naming both files.
- [ ] The bad fixture yields exactly one diagnostic per
  extra file sharing a value; the good fixture is clean.
- [ ] The new rule passes the per-rule allocation budget
  test.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
