---
title: Architecture principles
slug: hub
summary: >-
  SOLID and clean-architecture rules for
  mdsmith's Go core, TypeScript extension, and
  cross-system surfaces. Canonical home for
  the solid-architecture skill.
---
# Architecture principles

How SOLID and clean architecture apply to
mdsmith's actual code. The
[solid-architecture skill](../../../.claude/skills/solid-architecture/SKILL.md)
holds the generic language patterns; this
hub names mdsmith's packages, layers, and
the concrete anti-patterns we have already
hit.

## The five SOLID principles in mdsmith

- **Single responsibility**: every package
  under `internal/` answers one question.
  `internal/lint` answers "does this file
  violate a rule?"; `internal/fix` answers
  "what edits make it stop violating?";
  `internal/linkgraph` answers "what's a
  Markdown link and what does it point
  at?". They read each other's outputs but
  never collapse.
- **Open/closed**: new rules and fixes are
  added by creating a Go package under
  `internal/rules/<rule-name>/` (e.g.
  `internal/rules/linelength/`) plus a
  matching docs+fixtures directory at
  `internal/rules/MDS###-<rule-name>/`
  (e.g. `internal/rules/MDS001-line-length/`).
  The blank-import barrel
  `internal/rules/all/all.go` wires every
  production rule. The engine never needs
  edits when a rule is added.
- **Liskov substitution**: every
  `rule.Rule` implementation must work in
  every engine call site. A rule that
  needs the engine to know its name (so
  the engine special-cases it) is a Liskov
  violation — widen the interface or push
  the special case down into a config
  filter.
- **Interface segregation**: `internal/rule`
  exposes small interfaces (`Rule`,
  `FixableRule`, `Configurable`,
  `Defaultable`, `ListMerger`,
  `ConfigTarget`). A rule only implements
  the slice of capabilities it needs.
- **Dependency inversion**: high-level
  packages depend on the small `rule`
  interface, not on concrete rule
  packages. The VS Code extension talks to
  the LSP server over the wire protocol,
  not to a Go type.

## Project layering

Go side. `cmd/mdsmith` and `internal/lsp`
are both top-layer entry points. Both
depend on `pkg/mdsmith` — the public
`Session` engine API — never the reverse.
`Session` is the sanctioned orchestration
layer over `internal/engine`: it owns the
workspace, the compiled config, and the
cross-file and parse caches, and exposes
check, fix, and kinds (plan 219). The LSP
depends on it exclusively; the CLI routes
its check/fix path through it and reads a
few `internal/engine` result types
directly for output formatting.

```text
cmd/mdsmith              internal/lsp
  (CLI entry)              (LSP entry)
        └───────┐    ┌──────┘
                ↓    ↓
        pkg/mdsmith          (Session:
                              orchestration
                              entrypoint)
              └─> internal/engine
              └─> internal/{rule, fix,
                            config, output,
                            lint, linkgraph,
                            index,
                            discovery,
                            schema, …}
                    └─> internal/{mdtext,
                                  globpath,
                                  yamlutil,
                                  log,
                                  fieldinterp,
                                  placeholders}
                        (helpers; deps among
                         siblings are
                         allowed, cycles are
                         not — e.g.
                         placeholders →
                         fieldinterp)
                    └─> pkg/markdown
                        (the one goldmark
                         parse/produce
                         surface; public,
                         imports no
                         internal/ package)
                        └─> pkg/markdown/flavor
                            (extended-syntax
                             parsers, the
                             Flavor / Feature
                             support model,
                             and Detect;
                             public,
                             imports no
                             internal/ package)
                            └─> pkg/markdown/flavor/ext
                                (the five
                                 custom
                                 extensions)
                    └─> cue/types
                        (the embedded
                         field-type-shortcut
                         CUE library;
                         public, imports no
                         internal/ package;
                         internal/schema
                         reads its embed)
                    └─> cue/cuelite
                        (the public, versioned
                         CUE-subset façade;
                         imports no internal/
                         package; its eventual
                         consumers are schema,
                         requiredstructure,
                         query, fieldinterp,
                         and cuetemplate)
```

`pkg/markdown` sits at the bottom. It
owns the canonical CommonMark + PI
parse path. `internal/lint` and
`internal/release` depend on it. It
depends on nothing in the tree.

The `flavor` sub-package adds the
extended-syntax parsers. It owns every
custom goldmark parser in the tree.
The MDS034 rule and the schema engine
both build on it.

`cue/types` sits at the bottom too,
outside both `internal/` and `pkg/`. It
embeds the canonical
field-type-shortcut CUE library, the one
that lets a schema write `created: date`.
Its Go import path is
`github.com/jeduden/mdsmith/cue/types`,
mirroring how `pkg/markdown` is the public
parse surface. `internal/schema` reads
its embed to seed the runtime registry,
and the package imports nothing in the
tree.

`cue/cuelite` sits at the bottom too. It
is the public, versioned façade over the
small CUE subset mdsmith depends on. That
subset covers schema constraints, query
filters, placeholder paths, and catalog
templates. Its Go import path is
`github.com/jeduden/mdsmith/cue/cuelite`,
mirroring `cue/types`.

`cue/cuelite` lands first as a thin
wrapper over `cuelang.org/go`. It is
later flipped to a pure-Go engine behind
the same API. The CUE-backed path stays
as the differential oracle in the
module-internal `internal/cuelitetest`
harness — kept under `internal/` so the
`cuelang.org/go` import that plan 218
phase 4 deletes never becomes part of the
public surface. Its eventual
consumers are `internal/schema`,
`requiredstructure`, `query`,
`fieldinterp`, and `cuetemplate`. It
imports none of them, and nothing else in
the tree.

These packages are public surfaces.
For details see
[cross-system contracts](cross-system.md)
and [Public Markdown Library](../markdown-library.md).

Plus the rule plugins. Each rule has a Go
implementation package and a sibling
docs+fixtures directory:

```text
internal/rules/<rule-name>/         (Go pkg)
internal/rules/MDS###-<rule-name>/  (docs +
                                     good/,
                                     bad/)
  └─> internal/rule (interfaces only)
  └─> internal/mdtext (parse helpers)
  ✗ MUST NOT import internal/engine
  ✗ MUST NOT import another rule package
```

TypeScript side:

```text
editors/vscode/src/extension.ts   (entry)
  └─> wiring.ts                   (compose)
        └─> commands/*            (one each)
              └─> binary.ts       (find +
                                   exec)
              └─> commands/runner.ts
                                  (typed I/O
                                   to binary)
```

Cross-system contracts (LSP, CLI flags,
`.mdsmith.yml`, generated section markers,
plugin manifests, distribution shims) live
at the very edge. They are public APIs
under stricter compatibility rules. See
[cross-system contracts](cross-system.md).

## Test pyramid

Tests are part of the architecture.
mdsmith follows a four-layer pyramid
(unit, contract, integration, e2e)
and every function — exported and
unexported — ships with a dedicated
unit test. The canonical home is
[Test pyramid](tests.md); the
language pages bind the rule to
concrete file and symbol patterns.

## Anti-patterns we have actually hit

These are the patterns mdsmith audits have
caught and the reasons we reject them:

- **A new Go package named `util`,
  `common`, `helpers`, or `lib`.** The name
  answers no question, so the package
  attracts unrelated code.
- **A rule package importing another rule
  package.** Rules share helpers via
  `internal/mdtext` or
  `internal/rules/astutil`; reaching
  sideways into a sibling rule binds
  release cycles that should stay
  independent.
- **A Go or TypeScript command that
  parses Markdown inline** (a local
  `goldmark.New()`) instead of delegating
  to `pkg/markdown`. That pulls the
  parser config out of one place and into
  many — exactly the drift plan 163
  removed from `internal/release`.
- **A distribution shim (`npm/`,
  `python/`) that reimplements binary
  logic** instead of exec'ing it. Shims
  are Liskov substitutes for the binary;
  drift kills that property.
- **A `.mdsmith.yml` field consumed only
  inside one rule package** — promote it
  to that rule's settings or push the
  consumer back to where the data is
  owned, so the schema reflects ownership.
- **A test that mocks `rule.Rule`** instead
  of using a real Markdown fixture. Mocks
  at the boundary suggest the boundary is
  wrong; if the rule is hard to fixture-test,
  the rule's contract is too wide.
- **A TypeScript command that imports
  another command.** Commands share state
  through `wiring.ts`, not by reaching
  across.
- **A new public function in
  `internal/engine` whose only caller is
  one rule** — move it to `internal/rule`
  or `internal/mdtext`. Engine grows
  unbounded otherwise.
- **A new function landing without a
  dedicated unit test.** Either the
  function is too coupled to test in
  isolation (push it down to a port
  package so it can be tested) or the
  test was forgotten (write it). The
  audit flags uncovered functions as
  test debt.
- **An e2e test added where a unit
  test would do the same work.** E2E
  tests run the built binary; they
  are far slower than unit tests.
  Reserve them for full-binary
  behaviour the unit layer cannot
  reach.

## Sub-pages

<?catalog
glob:
  - "*.md"
  - "!index.md"
sort: title
header: |
  | Page | Description |
  |------|-------------|
row: "| [{title}]({filename}) | {summary} |"
?>
| Page                                               | Description                                                                                                                                                                           |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Architecture audit checklist](audit-checklist.md) | Checklist for sweeping origin/main for SOLID and boundary violations. Records findings in the audit log; schedules blockers as new plan files.                                        |
| [Cross-system contracts](cross-system.md)          | External-surface contracts: LSP, CLI, .mdsmith.yml, generated markers, plugin manifest, distribution shims. Public APIs.                                                              |
| [Go architecture patterns](go.md)                  | Go-specific SOLID and clean architecture patterns for mdsmith's cmd/ and internal/ packages.                                                                                          |
| [Test pyramid](tests.md)                           | Four-layer test pyramid (unit, contract, integration, e2e) and the rule that every function ships with a dedicated unit test. Included from the Go and TypeScript architecture pages. |
| [TypeScript architecture patterns](typescript.md)  | SOLID and clean architecture patterns for the mdsmith VS Code extension at editors/vscode/.                                                                                           |
<?/catalog?>

## When to consult this hub

- During plan generation in `plan/` — plans
  should respect the layering map.
- When designing a new `internal/` package
  or splitting an existing one.
- When wiring a new cross-system surface.
- During architecture audits of
  `origin/main` — see
  [audit checklist](audit-checklist.md).

The
[solid-architecture skill](../../../.claude/skills/solid-architecture/SKILL.md)
wraps this hub and the sibling pages with
agent-facing workflows for design, plan,
and audit modes.
