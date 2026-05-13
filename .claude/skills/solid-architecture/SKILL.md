---
name: solid-architecture
description: >-
  Apply SOLID principles and clean architecture to
  mdsmith's Go core (cmd/, internal/) and TypeScript
  VS Code extension (editors/vscode/). Use when
  generating or revising a plan in plan/ so the plan
  respects package boundaries; when designing a new
  internal/ package, splitting an existing one, or
  wiring a new cross-system surface (LSP, VS Code
  commands, npm/PyPI/asdf/mise shims, Claude plugin);
  and when auditing the main branch for accumulated
  boundary violations that should be scheduled as
  fixes. Trigger phrases include "design a package",
  "is this clean architecture", "refactor the
  boundary", "architecture audit", "plan a feature",
  "check main for arch debt", "review architecture",
  "solid principles".
user-invocable: true
argument-hint: "[mode: design | plan | audit]"
allowed-tools: >-
  Bash(git fetch:*), Bash(git log:*),
  Bash(git diff:*), Bash(git branch:*),
  Bash(mdsmith:*),
  Bash(go run ./cmd/mdsmith:*),
  LSP
---

# SOLID and clean architecture for mdsmith

The canonical [architecture hub][arch-hub]
holds the principles, the layering map, and
the anti-patterns. This skill wraps that
hub with three workflows: design, plan, and
audit.

Read the hub before any architectural
recommendation. The references below offer
depth per language and per surface.

## Prerequisites

- Run each fenced code block as its own
  Bash call. Do not chain with `&&`. Do not
  prefix with an inline variable assignment
  (`VAR=x cmd`) or wrap the leading token in
  `$(...)`. The skill's `allowed-tools` list
  matches on the command prefix, so the
  allowed token (e.g. `git log`) must be the
  first thing the shell sees.
- Where a step needs a value from a prior
  command (a SHA, a path), paste the literal
  value into the next command rather than
  storing it in a shell variable.

## Modes

Pass the mode as the skill argument
(`/solid-architecture design`, `… plan`, or
`… audit`). If no argument is supplied, infer
from the conversation: editing a plan file →
plan; writing new Go or TypeScript → design;
sweeping `origin/main` → audit.

### Design mode

Use design mode for code-shape decisions. It
fits any of:

- A new `internal/` package, or splitting an
  existing one.
- A new rule or fix variant.
- A new LSP capability.
- A new cross-system surface: VS Code
  command, distribution channel, plugin
  entry.

Workflow:

1. Identify which layer the change lives in.
   Cross-check against the layering map in
   the [architecture hub][arch-hub].
2. List the interface(s) the change crosses
   on a layer boundary. Confirm the
   dependency direction matches.
3. Walk the principle checklist in
   [Go patterns][arch-go] or
   [TypeScript patterns][arch-ts].
4. Reject package names that mix
   responsibilities (`util`, `common`,
   `helpers`, `lib`). Each package name must
   answer one question.
5. Write the failing test first (Red), then
   the smallest passing implementation
   (Green). This matches the TDD discipline
   in CLAUDE.md.

### Plan mode

Use when generating or revising a file under
`plan/`. Plans drive multi-step work and lock
in architectural decisions; getting the layer
boundaries wrong here ripples into every
commit that follows.

Workflow:

1. For each task in the plan, name the
   package(s) it touches and the layer(s) it
   crosses.
2. Mark any task that pushes a dependency the
   wrong direction (e.g. `internal/engine`
   importing `internal/rules/...`).
3. If a plan introduces a new interface,
   add a task for the test that locks the
   contract.
4. Annotate acceptance criteria with the
   SOLID principle each verifies, when one
   applies.
5. After editing, run
   `mdsmith fix PLAN.md` so the catalog
   table stays current (per CLAUDE.md).

### Audit mode

Use to sweep `origin/main` for accumulated
violations. Suited to a recurring schedule
via `/loop 1d /solid-architecture audit`.

Workflow:

1. Read the last audit checkpoint from
   `docs/development/architecture-audit.md`
   front matter (`audit-from:`). If the file
   does not exist, create it from the
   [audit checklist][arch-audit] §"Initial
   file" and start from one month back.
2. Diff that SHA against `origin/main` and
   walk the [audit checklist][arch-audit]
   over the touched files.
3. Append findings to the audit log with
   severity (`blocker`, `tax`,
   `nice-to-have`), the violating files, and
   the principle violated.
4. Do not fix in this run. Group blockers
   that share the same structural fix into
   one plan file under `plan/` referencing
   the audit entries; only split into more
   plans when a single one would exceed
   `mdsmith check`'s `max-file-length`. For
   tax and nice-to-have, leave entries in
   the audit log.
5. Update `audit-from:` to the current
   `origin/main` SHA.

## References

The canonical architecture docs live under
`docs/development/architecture/`:

- [Architecture hub][arch-hub] —
  principles, layering, anti-patterns.
- [Go patterns][arch-go] — SOLID in `cmd/`
  and `internal/`.
- [TypeScript patterns][arch-ts] — SOLID in
  the VS Code extension.
- [Cross-system contracts][arch-cross] —
  LSP, shims, plugins, config.
- [Audit checklist][arch-audit] — sweeping
  `origin/main`.

[arch-hub]: ../../../docs/development/architecture/index.md
[arch-go]: ../../../docs/development/architecture/go.md
[arch-ts]: ../../../docs/development/architecture/typescript.md
[arch-cross]: ../../../docs/development/architecture/cross-system.md
[arch-audit]: ../../../docs/development/architecture/audit-checklist.md

## Notes

- This skill never edits source code on its
  own. In design and plan modes it advises
  and rewrites prose; in audit mode it
  appends to
  `docs/development/architecture-audit.md`
  and creates plan files for blockers.
- The audit log is Markdown, so
  `mdsmith check .` will run over it; keep
  entries within the project's line-length
  budget (80 chars outside code blocks,
  tables, and URLs).
- When recommending a refactor, name the
  principle being upheld and the layer being
  protected. Vague "this would be cleaner"
  guidance is not actionable.
