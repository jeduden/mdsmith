---
id: 120
title: Unify glob matcher and field naming across mdsmith
status: "🔲"
model: sonnet
summary: >-
  Pick one glob library and one field name (`files:` vs
  `glob:`), migrate `ignore:`, `overrides:`,
  `kind-assignment:`, `<?catalog?>`, and CLI argument
  expansion to use them consistently, and deprecate the
  displaced surface.
---
# Unify glob matcher and field naming across mdsmith

## Goal

Use one glob matcher and one field name across every
glob surface in mdsmith. Config sections, directives,
and CLI argument expansion should accept the same
syntax. Users learn the rules once.

## Background

Today three independent glob systems coexist, summarized
in [docs/reference/globs.md](../docs/reference/globs.md):

| Surface                                                   | Matcher                | Field name | `!`-exclusion          |
|-----------------------------------------------------------|------------------------|------------|------------------------|
| `ignore:` / `overrides:.files` / `kind-assignment:.files` | `gobwas/glob`          | `files:`   | yes (added in plan 96) |
| `<?catalog?>`                                             | `doublestar`           | `glob:`    | yes                    |
| CLI argument expansion                                    | stdlib `filepath.Glob` | positional | no                     |

The split surfaces concretely as:

- `**` recursion works in `<?catalog?>` but is matcher-
  dependent for `ignore:` patterns.
- Brace expansion (`{a,b}`) works in `<?catalog?>` but
  not in config.
- The CLI accepts none of the above; users have to lean
  on shell expansion.
- The same list-of-patterns concept is called `files:`
  in some places and `glob:` in others.

Plan 96 added `!`-prefix exclusion to the config matcher
and documented the current state. Unifying the three
surfaces is a larger change that deserves its own plan.

## Design

### Matcher

`doublestar` is the most capable of the three. It
supports `**`, brace expansion, character classes, and a
well-defined matching algorithm. It already powers
`<?catalog?>` with the `!`-exclusion semantics plan 96
wired into config. Standardizing on it preserves catalog
behavior. Config and CLI gain a strict superset of what
they support today.

The shared matcher lives in a new package
(`internal/globpath` or a sibling) and exposes:

- `Match(pattern, path) bool` — single-pattern match
  with the same path/cleaned-path/basename fallbacks the
  config matcher uses today.
- `MatchAny(patterns, path) bool` — list match with
  `!`-prefix exclusion (the same semantics as plan 96's
  `globMatchAny`).
- `SplitIncludeExclude(patterns)` — for callers that
  need the split form (catalog uses this today).

### Field name

`glob:` is the better fit. It names the syntactic
artifact (a glob pattern) rather than the result (a
file list), and `<?catalog?>` and `<?include?>` already
use a glob-style vocabulary. `files:` is in older
config blocks but is the easier side to migrate because
it only appears in two keys (`overrides:`, `kind-
assignment:`).

Migration plan:

1. Add `glob:` as the canonical key on each block,
   accepting the same list shape.
2. Keep `files:` as a deprecated alias that loads into
   the same field; emit a deprecation warning when it is
   used.
3. Update `mdsmith init` and the docs to write `glob:`.
4. Schedule removal of `files:` for the release after
   the deprecation window.

### CLI argument expansion

Replace the `filepath.Glob` call in
[`resolveGlob`](../internal/lint/files.go) with the
shared matcher. CLI args now accept the same syntax as
config — including `**` and `!`-exclusion — and stop
silently failing on patterns the standard library
doesn't grasp.

## Tasks

1. Land the shared matcher package (new
   `internal/globpath`) with `Match`, `MatchAny`, and
   `SplitIncludeExclude`. Cover include-only, exclude-
   only, mixed-order, `**` recursion, and brace
   expansion in unit tests.
2. Migrate
   [`internal/config/ignore.go`](../internal/config/ignore.go)
   to call the shared matcher; remove the `gobwas/glob`
   dependency from `internal/config/`.
3. Migrate
   [`internal/rules/catalog/rule.go`](../internal/rules/catalog/rule.go)
   to call the shared matcher; remove its private
   `splitIncludeExclude`.
4. Migrate
   [`internal/lint/files.go:347`](../internal/lint/files.go)
   `resolveGlob` to call the shared matcher.
5. Add `glob:` as the canonical key on `overrides:` and
   `kind-assignment:` entries. Keep `files:` as a
   deprecated alias and surface a deprecation warning
   via `cfg.Deprecations`.
6. Update `mdsmith init` to emit `glob:`; update
   [docs/reference/globs.md](../docs/reference/globs.md),
   [docs/guides/file-kinds.md](../docs/guides/file-kinds.md),
   the `mdsmith help kinds` page, and any rule
   READMEs / fixtures that reference `files:`.
7. Add a regression test: a config that uses `files:`
   loads correctly and produces a deprecation warning;
   the same config rewritten with `glob:` produces no
   warning and the same effective rule config.
8. Drop `gobwas/glob` from `go.mod` once no callers
   remain.

## Acceptance Criteria

- [ ] Every glob surface in mdsmith — config, catalog,
      CLI argument expansion — resolves through one
      shared matcher; `gobwas/glob` and the stdlib
      `filepath.Glob` are no longer imported by
      production code.
- [ ] `**` recursion, brace expansion, and `!`-prefix
      exclusion behave identically across all three
      surfaces (covered by tests).
- [ ] `overrides:` and `kind-assignment:` accept
      `glob:` as the canonical key. `files:` continues
      to work and emits a deprecation warning naming
      the offending block.
- [ ] `mdsmith init` writes `glob:`; existing `files:`
      configs in the repo are migrated.
- [ ] [docs/reference/globs.md](../docs/reference/globs.md)
      collapses the three-surface table into a single
      surface and documents the deprecation timeline
      for `files:`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues

## ...

<?allow-empty-section?>
