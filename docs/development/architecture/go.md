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
- `internal/linkgraph` — the canonical
  Markdown link / directive / reference
  extractor. MDS027, the `mdsmith list
  backlinks` CLI, and the workspace
  symbol index (`internal/index`) all
  consult it so anchor normalisation,
  workspace-relative path resolution,
  and catalog-glob handling stay
  consistent across surfaces. The
  per-file extractor is pure (no file
  reads, no workspace walks) so callers
  can fan it out across goroutines.
- `internal/index` — the workspace
  symbol / edge graph (headings,
  link-ref defs, directives,
  front-matter keys, reverse edges);
  queried by the LSP, schema, and the
  rename / deps surfaces.
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
- `internal/index` is a peer support
  package both entry points may import;
  it must never import `internal/lsp`
  (plan 174 moved it out of
  `internal/lsp/index`).
- `internal/engine` may import
  `internal/rule`, never
  `internal/rules/...`.
- `internal/rules/...` may import
  `internal/rule`, `internal/mdtext`, and
  shared helpers; never `internal/engine`.
- The reverse (`engine` → `lsp`,
  `rule` → `engine`, `index` → `lsp`)
  is forbidden.

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

<?include
file: tests.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
### Test pyramid

mdsmith follows a four-layer test
pyramid. Each layer answers a
different question and sits in a
different place in the tree:

- **Unit** — one function or method
  per test. Lives next to source.
  No file I/O beyond inline string
  fixtures. Runs in milliseconds.
- **Contract** — locks a port-package
  interface or external surface
  shape. A contract test must fail
  loudly when the shape it pins
  drifts.
- **Integration** — multiple packages
  composed together against real
  Markdown fixtures.
- **E2E** — the built binary (or the
  packaged extension) against a
  fixture workspace.

The pyramid shape — many unit, fewer
contract, fewer integration, fewest
e2e — keeps the suite fast and the
feedback loop tight.

#### Every function has a dedicated unit test

A new function lands together with
its dedicated unit test by name.
Sub-behaviours of the same function
go in subtests under that parent.
The rule applies to exported and
unexported functions alike — in
production code. Test files
(`*_test.go`, `*.test.ts`) and
test-only helpers are out of scope:
the audit walks production sources
only and never asks for "tests for
tests". The audit flags every
production function in the touched
set that lacks a matching test.

The language-specific page binds
this rule to concrete file and
symbol patterns. For Go, that is
`TestFunctionName` for package
functions and `TestReceiver_Method`
for methods. For TypeScript, that
is a `describe("name")` block with
one or more `test("case")` cases
imported from `bun:test`.

#### Exemptions

A production function may skip its
dedicated test only if one of these
holds:

- It is generated code (file begins
  with a `// Code generated…` header,
  matches a generator file pattern
  such as `*_gen.go`, is a `*.d.ts`
  declaration, or is emitted under
  `dist/`). The file-level marker is
  sufficient — no per-function
  comment is required.
- It is a trivial accessor with no
  branch — a one-line getter or a
  `String()`-style format method.
  Add a one-line comment on the
  function so the audit can
  distinguish "no test by design"
  from "no test, forgotten".

#### Push down by default

A unit test on the same behaviour
is faster than the equivalent
integration test. It stays focused
on one function. It also survives
refactors better. The audit pushes
back on inverted pyramids:

- An integration test that exercises
  one function should move down to
  that function's own package as a
  unit test.
- An e2e test that exercises
  behaviour reachable through the
  integration layer should move down
  to integration.

Save e2e for the full process
boundary. Use it for exit codes.
Use it for signals. Use it for
subprocess lifecycle. Use it for
packaged-artifact tests.
<?/include?>

### Go-specific bindings

- **Unit tests** in `xxx_test.go`
  next to `xxx.go`. The dedicated
  test for a package function
  `func Foo` is `TestFoo`; for a
  method `func (r *Receiver) Foo`
  it is `TestReceiver_Foo`.
  Sub-behaviours may live as
  `t.Run("case", …)` subtests
  under that parent (e.g.
  `TestParseFrontMatterFields` in
  `internal/lint/frontmatter_test.go`)
  or as additional top-level
  functions
  `TestReceiver_Foo_Variant` (e.g.
  the `TestEngine_Check_*` family
  in
  `internal/archetype/gensection/engine_test.go`).
  Either form satisfies the
  dedicated-test rule — subtests
  when behaviours share setup,
  separate top-level when each
  variant stands alone.
- **Contract tests** in this repo:
  `internal/integration/rule_boundaries_test.go`
  pins the rule import graph;
  `internal/integration/directive_examples_test.go`
  pins the example-folder contract
  every directive rule must ship
  (good/bad/fixed and pattern/
  pairs, plus the registered rule
  ID surfaced by the directive).
  Add a new contract test whenever
  a new interface in
  `internal/rule/` or a new
  external surface shape lands.
- **Integration tests** live under
  `internal/integration/`. The rule
  fixture runner (`rules_test.go`)
  is the canonical example.
- **E2E tests** live under
  `cmd/mdsmith/` and are defined
  by behaviour — the test spawns
  the built `mdsmith` binary and
  drives it over stdio, exit
  code, or signals. Audit by what
  the test does, not the
  filename; three name shapes
  appear in the repo: `e2e_`
  prefix (`e2e_test.go`,
  `e2e_backlinks_test.go`),
  `_e2e_` suffix
  (`kinds_e2e_test.go`,
  `list_e2e_test.go`), and
  topic-named LSP subprocess
  tests (`lsp_test.go`,
  `lsp_hover_test.go`, …). The
  demo tape harness under `demo/`
  is also e2e.
- Rule fixtures live in
  `internal/rules/MDS###-<rule-name>/`:
  `good/` must lint clean against
  every default-enabled rule, `bad/`
  is excluded via `.mdsmith.yml`.
- Use `testify/require` for
  preconditions that abort the test;
  `testify/assert` for soft checks.
- Use `Same`/`NotSame` for pointer
  identity.
- Don't mock at the `rule.Rule`
  boundary — feed real Markdown via
  fixtures. A mock there is the
  smell of a too-wide contract.

Severity for missing unit tests:
`tax` by default. Promote to
`blocker` if the function sits on
a public surface — a `rule.Rule`
method, an LSP capability handler,
or a CLI subcommand entry.

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
- **A Go function with no matching
  test symbol in a sibling
  `*_test.go`.** Test debt; naming,
  severity, and exemptions per the
  Tests section above.
- **A test under
  `internal/integration/` that
  exercises a single function.**
  Pyramid is inverted; push the
  assertion down to a unit test in
  the function's own package.
- **A test that spawns the
  `mdsmith` binary as a
  subprocess to assert behaviour
  reachable without it.** Pyramid
  is inverted regardless of
  filename — any of the three
  e2e shapes above counts.
  Reserve e2e for behaviour that
  needs the full process
  boundary.

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
