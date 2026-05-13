---
id: 155
title: 'arch-fix: relocate convention types out of markdownflavor'
status: "🔲"
summary: >-
  Hoist Convention, RulePreset and ParseFlavor
  out of the markdownflavor rule into a layer
  internal/config can own, so config stops
  importing a rule package.
model: "sonnet"
depends-on: []
---
# arch-fix: relocate convention types

## Goal

Reverse the dependency inversion. Today
`internal/config` (a mid-layer) imports
`internal/rules/markdownflavor` (the
lowest layer). Convention and flavor
metadata is a config concept. It lives
inside a rule today and config reaches
down to fetch it.

## Context

Closes the second blocker in
[Audit 2026-05-13](../docs/development/architecture-audit.md).

[`internal/config/convention.go`](../internal/config/convention.go)
imports
`internal/rules/markdownflavor` to use
`Convention`, `RulePreset`,
`ParseFlavor`, `Lookup`, and
`ConventionNames`. The
[project layering map](../docs/development/architecture/index.md)
puts rules at the lowest layer. Config
sits between cmd/engine and the
helpers. The current direction is
`config → rules/...`. That is
inverted.

## Tasks

1. Create `internal/convention/` and move:

  - The `Convention`, `RulePreset`,
    and `Flavor` value types.
  - `ParseFlavor`, `Lookup`,
    `ConventionNames`, and the
    built-in convention registry.

2. Re-export from
   `internal/rules/markdownflavor`
   only the adapter that translates a
   `convention.Convention` into rule
   behavior. The rule consumes the
   data; it does not own it.
3. Update
   `internal/config/convention.go` to
   import `internal/convention`
   instead of
   `internal/rules/markdownflavor`.
4. Update any other consumer (search
   for the import) to import from the
   new location when the reference is
   to the data type, not the rule.
5. Add a regression test under
   `internal/config/convention_test.go`
   asserting the package compile-time
   imports contain no
   `internal/rules/...` paths.

## Acceptance Criteria

- [ ] `internal/convention/` exists.
  Its package comment states it owns
  convention and flavor data shapes
  independent of any rule. (SRP)
- [ ] Search reports no
  `internal/rules/` imports under
  `internal/config/`. (DIP /
  dependency direction)
- [ ] `internal/rules/markdownflavor`
  still compiles. It continues to
  expose its `rule.Rule` impl. It
  consumes the new package for data,
  not the other way round.
- [ ] All tests pass:
  `go test ./...`.
- [ ] `go tool golangci-lint run`
  reports no issues.
- [ ] `mdsmith check .` passes after
  the refactor.
- [ ] The audit entry for this
  blocker moves to a "Resolved by
  plan/155" section in
  [the audit log](../docs/development/architecture-audit.md).
