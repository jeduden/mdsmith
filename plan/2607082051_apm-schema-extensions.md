---
id: 2607082051
title: "Schema extensions: closed frontmatter and filename agreement"
status: "🔲"
model: opus
summary: >-
  Two MDS020 schema-engine features for APM's primitive contracts: a
  `frontmatter-closed: true` flag that flags undeclared frontmatter
  keys, and `\#(fmvar(name))` interpolation in the schema
  path/filename matcher. Opportunities A-4 and A-5.
depends-on: []
---
# Schema extensions for APM contracts

## Goal

Two APM rules have no home in mdsmith's
[schema grammar](../docs/reference/section-schema.md)
today. One rejects front-matter keys a schema does
not declare. The other makes a filename match a
front-matter field.

## Background

Two APM contracts have no mdsmith expression, both
verified against the schema engine:

- `.apm/prompts/*.prompt.md` preserves exactly five
  frontmatter keys (`description`, `input`,
  `allowed-tools`, `model`, `argument-hint`); `apm
  compile` drops every other key with a diagnostic,
  but only at compile time, after the file is written
  and shipped. Catching a dropped key at author time
  needs a closed frontmatter check, and mdsmith's
  `closed:` applies only to sections
  ([section-schema.md](../docs/reference/section-schema.md)),
  so on a frontmatter-only kind the setting is
  dropped.
- `.apm/skills/<name>/SKILL.md` requires the `name`
  field to equal the directory name. mdsmith's
  `path-pattern:` / `filename:` matcher validates the
  filename against a static glob, never against a
  frontmatter value.

Both are catalogued as A-4 and A-5 in the
[APM opportunity catalogue](../docs/research/apm-mdsmith/opportunities.md).
The `\#(fmvar(name))` interpolation already exists for
heading regexes in the schema engine, so feature (b)
extends a resolver that is already present rather than
adding one.

## Non-Goals

- New rules. Both features extend the existing
  [MDS020](../internal/rules/MDS020-required-structure/README.md)
  required-structure host rule.
- Changing the default. `frontmatter-closed` defaults
  to false; every existing schema keeps its current
  behavior.
- The APM kind pack itself. That is
  [plan 2607082050](2607082050_apm-coexist-guide-and-kind-pack.md),
  which consumes these features once they land.

## Tasks

1. Red/green: schema-parse and MDS020 tests for a
   `frontmatter-closed: true` flag that emits one
   diagnostic per frontmatter key absent from the
   `frontmatter:` map. Cover the multi-kind case
   (a file matching two kinds: the key is allowed if
   any kind declares it).
2. Implement `frontmatter-closed` on the frontmatter
   schema, defaulting to false and composing across
   kinds by union.
3. Red/green: matcher tests for `\#(fmvar(name))`
   inside `path-pattern:` / `filename:`, resolving the
   token from front matter before the filename
   comparison. Cover a match, a mismatch, and a
   missing field.
4. Implement `\#(fmvar(...))` interpolation in the
   path/filename matcher, reusing the existing `fmvar`
   resolver.
5. Document both keys in
   [section-schema.md](../docs/reference/section-schema.md)
   with an APM example for each.
6. Run `mdsmith fix PLAN.md` and `mdsmith check .`.

## Acceptance Criteria

- [ ] A kind with `frontmatter-closed: true` flags a
      frontmatter key not in its `frontmatter:` map,
      and stays silent when the flag is false or
      absent.
- [ ] A file matching two kinds accepts a key
      declared by either kind under
      `frontmatter-closed`.
- [ ] `path-pattern: ".apm/skills/\#(fmvar(name))/SKILL.md"`
      passes when `name` equals the directory and
      fails when it does not.
- [ ] A missing `name` field under an `fmvar` path
      matcher produces a clear diagnostic, not a
      panic.
- [ ] Both keys are documented with an APM example.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint
      run` reports no issues.
- [ ] `mdsmith check .` — 0 failures.
