---
title: Go architecture patterns
summary: >-
  Go-specific SOLID and clean architecture
  patterns for mdsmith's cmd/ and internal/
  packages.
---
# Go architecture patterns

Go-specific guidance for mdsmith's Go code.
The [architecture hub](index.md) covers the
cross-cutting principles; this page covers
how to apply each in Go.

## Single responsibility per package

A package is mdsmith's primary unit of
responsibility. Name the package after the
question it answers; keep it small enough
that the answer fits in one sentence.

Examples from this repo:

- `internal/config` — load and merge
  `.mdsmith.yml`.
- `internal/engine` — orchestrate rules
  over files.
- `internal/lint` — run rule checks.
- `internal/fix` — produce edits.
- `internal/lsp` — speak the Language
  Server Protocol.
- `internal/mdtext` — parse and walk
  Markdown.
- `internal/rule` — interfaces for rules
  and fixes.
- `internal/rules/<rule-name>/` — one Go
  rule per package (e.g.
  `internal/rules/linelength/`); docs and
  fixtures sit alongside at
  `internal/rules/MDS###-<rule-name>/`
  (e.g.
  `internal/rules/MDS001-line-length/`).

A package named `util` or `helpers` is
almost always a single-responsibility
violation: it answers "a grab bag of
things". Split it by question.

When you need a function in two packages,
ask: does this belong in the lower of the
two, or does it deserve its own package
named for the question it answers?

## Open/closed via plugin packages

The engine should not change when a rule is
added. The contract that enforces this:

- `internal/rule` defines the interfaces
  (`rule.Rule`, `rule.Fixer`, …).
- Each rule has a Go package at
  `internal/rules/<rule-name>/` and a
  sibling docs+fixtures directory at
  `internal/rules/MDS###-<rule-name>/`.
- Rules register themselves via `init()`;
  the blank-import barrel
  `internal/rules/all/all.go` wires every
  production rule into the registry. No
  `switch` in `engine` is keyed on rule ID.

When adding a new rule:

1. Create the Go package
   `internal/rules/<rule-name>/` with
   `rule.go` and `rule_test.go`.
2. Implement `rule.Rule` (and `rule.Fixer`
   if the rule is fixable).
3. Add a blank-import line for the new
   package in `internal/rules/all/all.go`.
4. Create the docs+fixtures directory
   `internal/rules/MDS###-<rule-name>/`
   with `README.md`, `good/`, and `bad/`.
5. The integration runner in
   `internal/integration/rules_test.go`
   discovers fixtures automatically.

If the new rule needs the engine to expose
new data, change the rule API in
`internal/rule`, not the rule's own
package. Extending the API is OCP-compliant;
reaching around it from the rule is not.

## Liskov: every Rule is interchangeable

Two recurring pitfalls in this codebase:

1. A rule that only works for certain
   `kind:` values. Kind selection lives in
   config layering, not in the rule. The
   rule sees what it is fed; if it receives
   the wrong input, that is an engine bug.
2. A rule that panics on edge cases the
   engine considers valid (empty document,
   pathological nesting, generated section
   markers). Return an error or a no-op
   diagnostic instead.

A rule that cannot honor the contract for
some inputs has a config problem, not a
code problem. Filter those inputs out in
config layering. Keep the rule unconditional.

## Interface segregation

The `rule` package defines small interfaces:

- `rule.Rule` — the base check.
- `rule.Fixer` — produce edits.
- `rule.ListMerger` — opt a list-typed
  setting into append-mode merging.
- `rule.SchemaContributor` — supply schema
  fragments.

Do not add a method to `rule.Rule` because
one rule wants it. Define a new interface
instead. The rule satisfies both. The
engine type-asserts when it needs the extra
capability:

```go
if m, ok := r.(rule.ListMerger); ok {
    mode = m.SettingMergeMode(key)
}
```

The same applies inside `internal/engine`:
prefer many small "ports" (interfaces the
engine consumes) over one wide one.

## Dependency inversion across layers

Go enforces dependency direction at compile
time via imports. Use that:

- `cmd/mdsmith` may import `internal/...`.
- `internal/engine` may import
  `internal/rule`, never
  `internal/rules/...`.
- `internal/rules/...` may import
  `internal/rule`, `internal/mdtext`, and
  shared helpers; never `internal/engine`.
- `internal/lsp` may import `internal/engine`
  and supporting packages; the reverse
  (`engine` → `lsp`) is forbidden.

A circular-import error from `go build` is
often the first sign of an inversion
violation. Do not break the cycle by moving
a type around; break it by inverting the
dependency through an interface.

When in doubt, draw the import graph: every
arrow should point downward in the layering
map. See the
[architecture hub](index.md).

## Clean architecture in `cmd/mdsmith`

`cmd/mdsmith` is wiring only:

- Parse flags.
- Construct the engine with its
  dependencies.
- Invoke a subcommand handler.
- Translate the result into an exit code
  and output stream.

Logic that belongs in `internal/engine` or
a domain package should not leak into
`cmd/`. A handler in `cmd/` longer than
~50 lines is a smell.

## Errors and panics

- Error messages: lowercase, no trailing
  punctuation (project style; see CLAUDE.md
  at the repo root).
- Prefer `fmt.Errorf("...: %w", err)` for
  wrapping; the caller decides how to
  surface.
- Reserve `panic` for invariants that, if
  violated, indicate a programming bug —
  impossible enum case, internal cache
  invariant. Never panic on user input.
- Don't return an error type whose only
  field is a string; reuse `errors.New` or
  `fmt.Errorf`. Define typed errors only
  when callers will inspect them.

## Testing patterns

- Unit tests next to the package
  (`xxx_test.go`).
- Integration tests in
  `internal/integration/`.
- Rule fixtures in
  `internal/rules/MDS###-<rule-name>/`:
  `good/` (must lint clean) and `bad/`
  (excluded via `.mdsmith.yml`).
- Use `testify/require` for preconditions
  that should abort the test;
  `testify/assert` for soft checks.
- Use `Same`/`NotSame` for pointer identity
  (see CLAUDE.md at the repo root).
- Don't mock at the `rule.Rule` boundary;
  feed real Markdown via fixtures.

## Common violations to flag

- A package outside `internal/rules/` that
  imports a specific rule's package.
- A type defined in `internal/engine` that
  is the public API for rules — should
  live in `internal/rule`.
- A function in `internal/engine` called
  only by rules — move it to
  `internal/rule` or `internal/mdtext`.
- A config field consumed only by one rule —
  move it to that rule's settings struct.
- A test that imports `internal/engine` to
  test a rule — push it down to a fixture.
- A new public method on the engine added
  to satisfy a single LSP capability —
  consider whether the LSP server can
  consume an existing engine output
  instead.
- A `Helper`, `Util`, or `Misc` symbol
  anywhere. The name is the problem.

## Refactor moves that usually work

When a Go-side architecture finding lands,
these moves resolve it more often than not:

- Push a leaky abstraction down: a type
  defined in a high layer but consumed
  only in low layers belongs in the low
  layer.
- Lift a shared dependency up to an
  interface: two implementations needed in
  the same place → define an interface and
  inject it.
- Split a package by question: a package
  whose top-level doc comment requires
  "and" to describe should become two.
- Replace a `switch` on type with method
  dispatch: a function in `engine` that
  switches on rule ID belongs as a method
  on `rule.Rule`.
