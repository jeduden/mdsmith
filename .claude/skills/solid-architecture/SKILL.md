---
name: solid-architecture
description: >-
  Apply SOLID principles and clean architecture to
  Go and TypeScript codebases. Use when generating
  or revising a multi-step plan so the plan respects
  package boundaries; when designing a new package,
  splitting an existing one, or wiring a new
  cross-system surface (LSP, CLI, distribution
  shim, plugin); and when auditing a branch for
  accumulated boundary violations that should be
  scheduled as fixes. Trigger phrases include
  "design a package", "is this clean architecture",
  "refactor the boundary", "architecture audit",
  "plan a feature", "check main for arch debt",
  "review architecture", "solid principles".
user-invocable: true
argument-hint: "[mode: design | plan | audit]"
allowed-tools: >-
  Bash(git fetch:*), Bash(git log:*),
  Bash(git diff:*), Bash(git branch:*),
  LSP
---

# SOLID and clean architecture workflow

The architecture content lives in the
project's docs — typically under
`docs/development/architecture/`. This
skill is the workflow that uses those
docs to keep the codebase consistent
with them.

In every mode below, the docs are the
source of truth. The skill reads them
and applies the rules they describe; it
does not carry its own copy of the
principles or patterns.

## Prerequisites

- Run each fenced code block as its own
  Bash call. Do not chain with `&&`. Do
  not prefix with an inline variable
  assignment (`VAR=x cmd`) or wrap the
  leading token in `$(...)`. The skill's
  `allowed-tools` list matches on the
  command prefix.
- Where a step needs a value from a prior
  command, paste the literal value into
  the next command rather than storing
  it in a shell variable.

## Modes

Pass the mode as the skill argument
(`/solid-architecture design`, `… plan`,
or `… audit`). If no argument is
supplied, infer from the conversation:
editing a plan or RFC document → plan;
writing new Go or TypeScript → design;
sweeping the main branch → audit.

### Design mode

Use design mode when:

- Shaping a new package.
- Splitting one whose responsibilities
  have drifted.
- Adding a feature at a layer boundary.
- Wiring a new external surface.

Workflow:

1. Read the project's architecture hub
   (commonly
   `docs/development/architecture/index.md`)
   for the layering map.
2. Read the language-specific page —
   `go.md` for Go code, `typescript.md`
   for TypeScript code — to find the
   patterns the change must respect.
3. Identify which layer the change
   lives in. Confirm the dependency
   direction matches what the docs say.
4. Walk the docs' SOLID checklist
   (single responsibility, open/closed,
   Liskov, interface segregation,
   dependency inversion). Flag any
   shape the docs reject.
5. Write the failing test first (Red),
   then the smallest passing
   implementation (Green).

If the docs are silent on the case
you're designing, that's a gap. Surface
it and propose the doc update before
writing the code.

### Plan mode

Use when generating or revising a
multi-step plan, RFC, or task list.

Workflow:

1. Read the project's architecture hub
   and the language-specific pages
   relevant to the plan's surface.
2. For each task in the plan, name the
   package(s) or module(s) it touches
   and the layer(s) it crosses.
3. Mark any task that pushes a
   dependency in a direction the docs
   forbid.
4. If a plan introduces a new
   interface, add a task for the test
   that locks the contract.
5. Annotate acceptance criteria with
   the SOLID principle each verifies,
   when one applies.
6. After editing, run the project's
   plan regeneration step (e.g. a
   catalog refresh) so plan indices
   stay current.

### Audit mode

Use to sweep the main branch for
accumulated violations against what the
docs say. Suited to a recurring schedule
(e.g. via `/loop 1d /solid-architecture
audit`).

Follow the steps in
[`audit-checklist.md`](audit-checklist.md)
exactly. Every check there cites the
project's architecture docs as the rule
source.

## Notes

- This skill never edits source code on
  its own. In design and plan modes it
  advises and rewrites prose; in audit
  mode it appends to the project's
  audit log and creates plan files for
  blockers.
- When the docs and the code disagree,
  one of them is wrong. Surface the
  conflict explicitly; do not let the
  skill silently treat the code as
  authoritative.
- When recommending a refactor, cite
  the doc section that justifies it.
  Vague "this would be cleaner"
  guidance is not actionable.

## Architecture doc references

The link definitions below resolve the
slug labels used throughout this skill
(e.g. `[hub]`, `[go]`, `[ts]`). The
`<?catalog?>` directive regenerates the
block from each canonical doc's `slug`
front-matter field — never hand-edit
the entries.

<?catalog
glob: "../../../docs/development/architecture/*.md"
sort: slug
row: "[{slug}]: {filename}"
?>
[audit]: ../../../docs/development/architecture/audit-checklist.md
[cross]: ../../../docs/development/architecture/cross-system.md
[go]: ../../../docs/development/architecture/go.md
[hub]: ../../../docs/development/architecture/index.md
[tests]: ../../../docs/development/architecture/tests.md
[ts]: ../../../docs/development/architecture/typescript.md
<?/catalog?>
