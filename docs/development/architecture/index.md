---
title: Architecture principles
summary: >-
  SOLID principles and clean architecture rules
  for mdsmith's Go core, TypeScript VS Code
  extension, and cross-system integration
  surfaces. Canonical home; the
  solid-architecture skill includes this page.
---
# Architecture principles

Keep mdsmith's Go core, TypeScript extension,
and integration surfaces aligned with SOLID
and clean architecture.

The Go linter lives in `cmd/` and `internal/`.
The VS Code extension lives in
`editors/vscode/`. The distribution shims
live in `npm/` and `python/`. The Claude
plugin lives in `.claude-plugin/`. Each layer
depends on stable contracts. Drift is
expensive to unwind later.

This page holds the cross-cutting principles
and the project layering map. Language and
surface-specific depth lives in the sub-pages
listed at the bottom.

## The five SOLID principles

The five SOLID principles, as concrete
constraints in this codebase:

- **Single responsibility**: every package
  under `internal/` answers one question.
  `internal/lint` answers "does this file
  violate a rule?"; `internal/fix` answers
  "what edits make it stop violating?". Do
  not collapse them because a function feels
  shared.
- **Open/closed**: new rules and fixes are
  added by creating a Go package under
  `internal/rules/<rule-name>/` (e.g.
  `internal/rules/linelength/`) and a
  matching docs+fixtures directory at
  `internal/rules/MDS###-<rule-name>/`
  (e.g. `internal/rules/MDS001-line-length/`).
  The blank-import barrel
  `internal/rules/all/all.go` is the
  central registry — no edits to the engine
  are needed. New conventions extend via
  deep-merge config layers, not new
  switches in `internal/engine`.
- **Liskov substitution**: every `rule.Rule`
  implementation must work in every engine
  call site without special-casing. A rule
  that needs the engine to know its name is
  a Liskov violation — widen the interface
  or push the special case down.
- **Interface segregation**: the `rule`
  package defines small interfaces (`Rule`,
  `Fixer`, `ListMerger`, …) so a rule only
  depends on the capabilities it uses. Do
  not add methods to `Rule` because one rule
  wants them.
- **Dependency inversion**: high-level code
  depends on interfaces, not concretes. The
  engine talks to `rule.Rule`, never to a
  specific rule package. The VS Code
  extension talks to the LSP server over
  the protocol, never to a Go type.

## Project layering

Dependencies flow downward only. A higher
layer may import a lower one; the reverse is
a violation.

Go side:

```text
cmd/mdsmith                (entry, wiring)
  └─> internal/engine      (orchestration)
        └─> internal/{rule, fix, config,
                      output, lint, lsp,
                      discovery, schema, …}
              └─> internal/{mdtext, globpath,
                            yamlutil, log,
                            placeholders}
                  (pure helpers; no
                   cross-deps among siblings)
```

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

Cross-system contracts live at the very edge.
They include the LSP wire protocol, the
`.mdsmith.yml` schema, generated section
markers, the plugin manifest, and shim entry
points. They follow their own versioning
rules. See
[cross-system contracts](cross-system.md).

## Anti-patterns to reject on sight

- A new Go package named `util`, `common`,
  `helpers`, or `lib`.
- A rule package that imports another rule
  package.
- A command (TS or Go) that builds and
  parses Markdown inline instead of
  delegating to `internal/mdtext`.
- A distribution shim (`npm/`, `python/`)
  that reimplements binary logic instead of
  exec'ing it.
- A `.mdsmith.yml` field consumed only
  inside one rule package — promote it to
  that rule's settings or push the consumer
  back to where the data is owned.
- A test that mocks `rule.Rule` instead of
  using a real Markdown fixture; mocks at
  the interface boundary suggest the
  boundary is wrong.
- A TypeScript command that imports another
  command.
- A new public function in `internal/engine`
  whose only caller is one rule — move it to
  `internal/rule` or `internal/mdtext`.

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
| Page                                               | Description                                                                                                                                                                                                                                   |
|----------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Architecture audit checklist](audit-checklist.md) | Concrete checklist for sweeping origin/main for SOLID and boundary violations. Records findings in the architecture audit log and schedules blockers as new plan files under plan/.                                                           |
| [Cross-system contracts](cross-system.md)          | Architecture rules for mdsmith's external surfaces — LSP wire protocol, CLI flags, .mdsmith.yml schema, generated section markers, plugin manifest, and distribution shims. Public APIs with stricter compatibility rules than internal code. |
| [Go architecture patterns](go.md)                  | Go-specific SOLID and clean architecture patterns for mdsmith's cmd/ and internal/ packages.                                                                                                                                                  |
| [TypeScript architecture patterns](typescript.md)  | SOLID and clean architecture patterns for the mdsmith VS Code extension at editors/vscode/.                                                                                                                                                   |
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
wraps these docs with agent-facing
workflows for design, plan, and audit
modes.
