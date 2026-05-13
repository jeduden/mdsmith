---
title: Go architecture patterns
slug: go
summary: >-
  Go-specific SOLID and clean architecture
  patterns for mdsmith's cmd/ and internal/
  packages.
---
# Go architecture patterns

SOLID and clean-architecture rules for
mdsmith's Go code. This page is the
source of truth — it names the actual
packages, shows the shapes we use, and
explains why. The
[solid-architecture skill](../../../.claude/skills/solid-architecture/SKILL.md)
reads this page in design and audit
modes to check Go changes against it.

## Single responsibility per package (SRP)

Each `internal/` package answers one
question. The current production set:

- `internal/config` — load and merge
  `.mdsmith.yml` across defaults, kinds,
  and overrides.
- `internal/engine` — orchestrate rules
  over files; owns the run loop.
- `internal/lint` — run rule checks
  against a parsed file.
- `internal/fix` — produce edits that
  make a file stop violating rules.
- `internal/lsp` — speak the Language
  Server Protocol; consumes the engine.
- `internal/mdtext` — parse and walk
  Markdown; the only place that knows
  goldmark.
- `internal/rule` — interfaces for rules
  and fixes (the ports package).
- `internal/rules/<rule-name>/` — one Go
  rule per package, e.g.
  `internal/rules/linelength/`. Docs and
  fixtures live alongside in
  `internal/rules/MDS###-<rule-name>/`
  (e.g.
  `internal/rules/MDS001-line-length/`).

The names answer the question the package
exists to answer. A package named `util`
fails that test — it answers "a grab bag",
so unrelated code accumulates.

## Open/closed via plugin packages (OCP)

The engine never changes when a rule is
added. The contract:

1. Create the Go package
   `internal/rules/<rule-name>/` with
   `rule.go` and `rule_test.go`.
2. Implement `rule.Rule` (and
   `rule.FixableRule` — the
   `Fix(f *lint.File) []byte` method — if
   the rule is fixable).
3. Add a blank-import line for the new
   package in
   `internal/rules/all/all.go`. That
   barrel registers every production rule
   in `init()`.
4. Create the docs+fixtures directory
   `internal/rules/MDS###-<rule-name>/`
   with `README.md`, `good/` (must lint
   clean), and `bad/` (excluded via
   `.mdsmith.yml`).
5. The integration runner in
   `internal/integration/rules_test.go`
   discovers fixtures automatically.

If the rule needs the engine to expose
new data, change the interface in
`internal/rule`. Do not reach upward from
the rule package. Do not widen the
engine's API in `internal/engine`.

## Liskov substitution (LSP)

Every `rule.Rule` implementation must
work in every engine call site. Two
recurring traps:

1. A rule that only works for certain
   `kind:` values. The selection lives
   in config layering, not in the rule.
   The rule sees what it is fed; if
   it receives the wrong input, that is
   an engine bug.
2. A rule that panics on edge cases the
   engine considers valid (empty
   document, pathological nesting,
   generated section markers). Return
   an error or a no-op diagnostic
   instead.

A rule that cannot honor the contract
for some inputs has a config problem,
not a code problem. Filter those inputs
out in config layering. Keep the rule
unconditional.

## Interface segregation (ISP)

`internal/rule` exposes one base
interface plus narrow capability
interfaces. A rule implements only the
ones it satisfies; the engine type-asserts
when it needs the extra capability.

- `rule.Rule` — the base check.
- `rule.FixableRule` — emit edits via
  `Fix(f *lint.File) []byte`.
- `rule.Configurable` — accept
  user-tunable settings.
- `rule.Defaultable` — override the
  default enabled state.
- `rule.ListMerger` — opt a list-typed
  setting into append-mode merging.
- `rule.ConfigTarget` — validate
  `.mdsmith.yml` itself, not Markdown.

```go
if m, ok := r.(rule.ListMerger); ok {
    mode = m.SettingMergeMode(key)
}
```

This keeps `rule.Rule` narrow for rules
that only do the base check. Fix-capable
or config-validating rules participate by
implementing extra interfaces. No rule is
forced into a wide surface it does not
need.

## Dependency inversion across layers (DIP)

The compiler enforces it. The arrows that
must hold:

- `cmd/mdsmith` may import `internal/...`.
- `internal/lsp` may import
  `internal/engine` and its support
  packages.
- `internal/engine` may import
  `internal/rule`, never
  `internal/rules/...`.
- `internal/rules/...` may import
  `internal/rule`, `internal/mdtext`, and
  shared helpers; never `internal/engine`.
- The reverse (`engine` → `lsp`,
  `rule` → `engine`) is forbidden.

A circular-import error from `go build`
is usually the first sign of an inversion
violation. Don't break the cycle by
moving the type that triggered it; break
it by inverting the dependency through an
interface in `internal/rule` (or the
appropriate ports package).

## Clean wiring in `cmd/mdsmith`

The CLI entry does flag parsing,
constructs the engine with its
dependencies, invokes a subcommand
handler, and translates the result into
an exit code and output stream. Anything
domain-related — including how files are
discovered, how diagnostics are merged,
how plans are validated — belongs in
`internal/engine` or its dependencies. A
handler in `cmd/mdsmith` longer than
~50 lines is a smell.

## Errors and panics

- Error messages are lowercase, no
  trailing punctuation (standard Go
  style; see CLAUDE.md at the repo root).
- Wrap with `fmt.Errorf("...: %w", err)`;
  the caller decides how to surface.
- `panic` is reserved for invariants
  that, if violated, mean a programming
  bug — impossible enum case, internal
  cache invariant. Never panic on user
  input.
- A typed error whose only field is a
  string adds no value; reuse
  `errors.New` or `fmt.Errorf`. Define
  typed errors only when callers will
  inspect them.

## Tests

- Unit tests next to the package
  (`xxx_test.go`).
- Cross-package tests in
  `internal/integration/`.
- Rule fixtures live in
  `internal/rules/MDS###-<rule-name>/`:
  `good/` must lint clean against every
  default-enabled rule, `bad/` is
  excluded via `.mdsmith.yml`.
- Use `testify/require` for
  preconditions that abort the test;
  `testify/assert` for soft checks.
- Use `Same`/`NotSame` for pointer
  identity.
- Don't mock at the `rule.Rule` boundary
  — feed real Markdown via fixtures. A
  mock there is the smell of a too-wide
  contract.

## Common violations to flag

These are mdsmith-specific instantiations
of the general anti-patterns in the
skill. We list them so the audit
checklist can pattern-match.

- **A package outside `internal/rules/`
  importing a specific rule package.**
  E.g. `internal/lint` reaching for
  `internal/rules/linelength`. The
  consumer should go through
  `internal/rule` so the rule set stays
  swappable.
- **A type defined in `internal/engine`
  that is the public API for rules.**
  Belongs in `internal/rule` instead, so
  the engine can change without breaking
  every rule import.
- **A function in `internal/engine`
  called only by rules.** Move it down to
  `internal/rule` or `internal/mdtext` so
  the engine is not a dumping ground.
- **A `.mdsmith.yml` field consumed only
  by one rule.** Move it into that rule's
  `settings` struct so ownership is
  visible in code review.
- **A test that imports `internal/engine`
  to test a rule.** Push it to a fixture
  under the rule's `good/` or `bad/`
  directory; the integration runner picks
  it up automatically.
- **A new public method on the engine
  added to satisfy one LSP capability.**
  Consider whether the LSP server can
  consume an existing engine output;
  widening the engine's API for a single
  caller couples the two layers harder
  than they need to be.
- **A `Helper`, `Util`, or `Misc` symbol
  anywhere.** The name is the problem;
  rename until it answers a question.

## Refactor moves we have used

- Push a leaky abstraction down. Several
  types defined in `internal/engine` have
  moved to `internal/rule` once we
  noticed only rules consumed them.
- Lift a shared dependency up to an
  interface. The `Configurable` and
  `ListMerger` interfaces in
  `internal/rule` started as duplicated
  helper code and were lifted once two
  rules needed the same shape.
- Split a package by question. If the
  package doc comment requires "and" to
  describe ("loads config and applies
  overrides and validates schemas"), the
  package wants to be two.
- Replace a `switch` on rule ID with
  method dispatch. A function in
  `internal/engine` that switches on
  rule name belongs as a method on
  `rule.Rule` or as a new capability
  interface in `internal/rule`.
