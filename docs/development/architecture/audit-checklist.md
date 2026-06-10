---
title: Architecture audit checklist
slug: audit
summary: >-
  Checklist for sweeping origin/main for
  SOLID and boundary violations. Records
  findings in the audit log; schedules
  blockers as new plan files.
---
# Architecture audit checklist

The
[solid-architecture skill][skill-audit]
holds the generic audit workflow.

This page is the mdsmith-specific
binding. It pins down the audit log
path, the plan numbering convention,
the lint command that refreshes parent
catalogs, and the project's lint
budget.

[skill-audit]: ../../../.claude/skills/solid-architecture/audit-checklist.md

## mdsmith-specific bindings

- **Audit log location**:
  `docs/development/architecture-audit.md`.
  Created on the first run from the
  skill checklist's "Initial file"
  template.
- **`audit-from:` front-matter field**:
  the SHA the next sweep starts from.
  Updated at the end of every audit.
- **Plan directory**: `plan/` with a
  numeric prefix
  (`plan/<id>_arch-fix-<slug>.md`).
  The id is the minute-precision UTC
  creation time, `date -u +%y%m%d%H%M`;
  if that id is taken, add one minute.
  Never allocate "highest prefix plus
  one" — the allocation contract lives
  in [plan/proto.md](../../../plan/proto.md).
- **Plan status sentinel**: `🔲` for
  "not started" (see
  [plan/proto.md](../../../plan/proto.md)).
- **Lint command after recording**: run
  `mdsmith fix .` from the workspace
  root so the audit-log entry refreshes
  in the parent catalogs in CLAUDE.md,
  AGENTS.md, PLAN.md, and
  `.github/copilot-instructions.md`.
- **Line-length budget for log
  entries**: 80 characters outside code
  blocks, tables, and URLs (the project
  default in `.mdsmith.yml`).
- **Readability budget**: bullet lists
  beat dense paragraphs for the
  per-finding "suggested fix". Single
  sentences over ~30 words trip MDS023 /
  MDS024 on the audit log.

## Initial file template

```markdown
---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-
  architecture findings on origin/main.
  The solid-architecture skill (audit
  mode) appends here; blockers are also
  filed as plans.
audit-from: <commit SHA one month ago>
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.

## Audit YYYY-MM-DD (range:
<from-sha>..<to-sha>)

### blockers

### tax

### nice-to-have
```

If the one-month-back lookup returns
empty on this repo, fall back to the
root commit:

```bash
git rev-list --max-parents=0 origin/main
```

Use the first SHA from that output as
the baseline. (At the time of writing,
the mdsmith repo is younger than a
month, so every audit so far has used
this fallback.)

## Walking the checklist

Follow the steps in
[the skill's audit checklist][skill-audit]
exactly. The skill describes the
generic workflow:

1. Refresh the checkpoint (with the
   shell-variable warnings the skill
   spells out).
2. Walk the language-level layering
   checks — Go and TypeScript — using
   [Go patterns](go.md) and
   [TypeScript patterns](typescript.md)
   on this repo.
3. Walk the cross-system contract
   checks against
   [cross-system contracts](cross-system.md).
4. Apply the severity rubric.
5. Append findings to the audit log
   under a new `## Audit YYYY-MM-DD`
   heading.
6. Group blockers by the structural fix
   they share into one plan each.
7. Tell the user what was found and
   what was scheduled.

## Test audit bindings

Architecture audits also check test
coverage. The
[Test pyramid](tests.md) doc is the
source of truth; the language pages
([Go](go.md), [TypeScript](typescript.md))
include it and add file-pattern
bindings. The mdsmith-specific knobs
the audit needs are below.

- **Unit-test location**: `xxx_test.go`
  next to `xxx.go` for Go;
  `xxx.test.ts` next to `xxx.ts` for
  the VS Code extension.
- **Function-coverage rule**: every
  Go and TypeScript production
  function — both exported and
  unexported — has a dedicated test
  by name (`TestFoo` for Go package
  `func Foo`, `TestReceiver_Foo`
  for a method on `Receiver`; a
  `describe("foo")` block with one
  or more `test(…)` cases imported
  from `bun:test` for TS `foo`).
  Test files (`*_test.go`,
  `*.test.ts`) and test-only
  helpers are out of scope — the
  audit walks production sources
  only. Generated files (`*_gen.go`,
  `*.d.ts`, `dist/`) and trivial
  accessors with no branch are
  exempt; see
  [Test pyramid §"Exemptions"](tests.md#exemptions).
- **Contract tests** for Go in this
  repo live under
  `internal/integration/` rather
  than alongside the port-package
  they pin. Examples:
  `internal/integration/rule_boundaries_test.go`,
  `internal/integration/directive_examples_test.go`.
- **Integration test location**:
  `internal/integration/` for Go.
  TypeScript integration tests sit
  next to the command module they
  exercise.
- **E2E test location**: under
  `cmd/mdsmith/` for Go, defined
  by behaviour (the test spawns
  the built binary and drives it
  over stdio, exit code, or
  signals). Three filename shapes
  appear in the repo, all e2e:
  `e2e_` prefix (`e2e_test.go`,
  `e2e_backlinks_test.go`),
  `_e2e_` suffix
  (`kinds_e2e_test.go`,
  `list_e2e_test.go`,
  `explain_e2e_test.go`), and
  topic-named LSP subprocess
  tests (`lsp_test.go`,
  `lsp_hover_test.go`, …). Demo
  tapes under `demo/` are also
  e2e. The VS Code extension host
  runs are e2e for the TypeScript
  side.
- **Severity for missing unit
  test**: `tax` by default;
  `blocker` if the function is on a
  public surface (a `rule.Rule`
  method, an LSP capability handler,
  a CLI subcommand entry, an
  exported VS Code command).

## mdsmith-specific checks worth flagging

These show up enough that they deserve
explicit mention here:

- **A rule package importing another
  rule package** — always a DIP
  blocker. Helpers belong in
  `internal/mdtext` or
  `internal/rules/astutil`.
- **`cmd/mdsmith/main.go` past ~1000
  lines** — handler bodies have crept
  in; relocate to `internal/engine` or
  a per-subcommand file.
- **`internal/lsp/server.go` or
  `symbols.go` past ~1000 lines** —
  split along the dispatch groups.
- **A `.mdsmith.yml` field reachable
  from only one rule** — push it into
  that rule's settings struct.
- **A new public method on
  `internal/engine` added to satisfy
  one LSP capability** — consider
  consuming an existing engine output
  instead.
- **A test that imports
  `internal/engine` to test a rule** —
  push it to a fixture under the
  rule's `good/` or `bad/` directory.
- **A Go function with no matching
  test symbol in a sibling
  `*_test.go`** — `TestFoo` for a
  package function `func Foo`,
  `TestReceiver_Foo` (or
  `TestReceiver_Foo_Variant`) for a
  method on `Receiver`. Test debt;
  severity per the rule above.
- **A TypeScript function not
  covered by a `describe("name",
  () => { test(…) })` block (with
  `describe`/`test` imported from
  `bun:test`) in a sibling
  `*.test.ts`** — same rule for the
  extension.
- **A test under
  `internal/integration/` that
  exercises a single function** —
  pyramid is inverted; push the
  assertion down to a unit test in
  the function's own package.
- **A test that spawns the
  `mdsmith` binary as a
  subprocess to assert behaviour
  reachable without it** —
  pyramid is inverted regardless
  of the filename shape; e2e
  tests build and run the binary,
  reserve them for behaviour that
  needs the full
  process boundary.

## Common skip cases in this repo

- Files under `testdata/` and
  `internal/rules/MDS###-<rule-name>/{good,bad}/`
  are fixtures; their architecture is
  by design.
- Generated section bodies (between
  `<?directive?>` markers) are
  auto-produced; review the directive
  parameters, not the body.
- Comments-only changes do not require
  an audit entry unless they document
  a contract that has changed.
- Vendored or generated Go code
  (`*_gen.go`, code under
  `internal/…/gen`) is excluded.
