---
title: Audit workflow
summary: >-
  Step-by-step audit workflow. Reads the
  project's architecture docs as the rule
  source; records findings in the audit
  log and schedules blockers as plans.
---
# Audit workflow

Use this checklist to sweep a branch for
SOLID and boundary violations. The
project's architecture docs are the rule
source — every check below cites them.
Do not encode architectural rules here;
if a rule is missing, fix the docs first
and re-run.

For each finding capture:

- File(s) involved.
- Principle violated (SRP / OCP / LSP /
  ISP / DIP) or layering rule, with the
  doc section that names it.
- Severity: `blocker`, `tax`, or
  `nice-to-have`.
- Suggested fix — a short sentence, or
  a bullet list if a single sentence
  would trip the project's readability
  rules on the audit log.

## Step 0: locate the project bindings

Open the project's audit-checklist page.
It usually sits at
`docs/development/architecture/audit-checklist.md`.
That page lists the project bindings.
You will use: the audit-log path, the
plan numbering rule, the lint command,
and the lint budget.

If the project's audit log does not
exist yet, create it. Use the
"Initial file" template the project
documents. Fall back to a generic
"Architecture audit log" layout
otherwise.

## Step 1: refresh the checkpoint

Read the last audit SHA from the
project's audit log front matter
(`audit-from:`). Call it `<from-sha>`
below. Paste the literal SHA into each
command rather than capturing it into a
shell variable — the allowed-tools
allowlist matches on the command's
leading token, so a `VAR=… cmd` prefix
or `$(…)` capture would block the call.

Fetch:

```bash
git fetch origin main
```

List the files touched since the
checkpoint — replace `<from-sha>` with
the literal SHA before running:

```bash
git log --name-only --pretty=format: <from-sha>..origin/main
```

Keep the file list as the "touched set"
for the rest of this audit; do not
assign it to a shell variable.

If the one-month-back lookup returns
empty on a young repo, fall back to the
root commit:

```bash
git rev-list --max-parents=0 origin/main
```

When auditing in a worktree, first
verify the worktree has the project
architecture docs by listing the docs
directory. Concurrent worktree creation
can race; catch the mismatch before
walking.

## Step 2: walk the docs over the touched set

For each source file in the touched
set, consult the docs page that covers
its language or surface, and flag any
shape the docs reject.

- `.go` files → the project's Go
  architecture page (commonly
  `docs/development/architecture/go.md`).
  Walk every SOLID section, the
  "Tests" section, and the "Common
  violations to flag" list.
- `.ts` / `.tsx` files → the project's
  TypeScript architecture page. Walk
  every SOLID section, the wiring
  section, the "Tests" section, and
  the "Common violations to flag"
  list.
- Files touching wire protocols, CLI
  flags, config schemas, generated
  markers, plugin manifests, shims, or
  any other public surface → the
  project's cross-system page. Apply
  the boundaries table and the
  versioning rules.

For every flagged shape, record the
section that justifies the flag.

### Test pyramid sub-walk

The language pages include a
canonical "Test pyramid" partial.
It commonly lives at
`docs/development/architecture/tests.md`.
The partial defines four layers:
unit, contract, integration, and
e2e. It also sets the rule that
every function ships with a
dedicated unit test.

For every production function in the
touched set — skip `*_test.go`,
`*.test.ts`, and test-only helper
files; the audit does not ask for
"tests for tests":

- Confirm a dedicated unit test
  exists in the matching test file,
  named after the function per the
  language page's binding (e.g.
  `TestFunctionName` or
  `TestReceiver_Method` for Go;
  `describe("name")` containing
  `test(…)` cases from `bun:test`
  for TS).
- If the function lives in a
  generated file (`*_gen.go`,
  `*.d.ts`, anything emitted under
  `dist/`, or any file beginning
  with a `// Code generated…`
  header), the file-level marker
  is enough — do not flag and do
  not require a per-function
  exemption comment.
- If the function is a trivial
  accessor with no branch (a
  one-line getter or
  `String()`-style format method),
  confirm a one-line exemption
  comment is present so the audit
  can distinguish "no test by
  design" from "no test,
  forgotten".
- If a test exists but lives one
  layer too high (an integration
  test that drives a single
  function, an e2e test that
  duplicates integration), record
  an inverted-pyramid finding.

Record missing tests as findings.
Use the severity rule the project
sets. See its architecture
audit-checklist page for the
project's "Severity for missing
unit test" entry.

## Step 3: severity rubric

- **blocker** — violates dependency
  direction, breaks a published
  contract, or makes a future refactor
  materially harder. Schedule a fix
  before the next release. File a plan
  with a back-link to the audit entry.
- **tax** — works today but adds
  friction (a function in the wrong
  package, a one-off setting).
  Schedule inside the next plan cycle;
  leave the entry in the audit log
  until picked up.
- **nice-to-have** — cosmetic
  (naming, small duplication, doc
  tightening). Batch into a cleanup
  commit when convenient.

## Step 4: record findings

Append to the audit log under a new
`## Audit YYYY-MM-DD (range: …)`
section. Order entries by severity
(`### blockers`, `### tax`, `###
nice-to-have`). For each entry, cite
the doc section that justifies the
flag:

```markdown
- `path/to/file`: imports a sibling
  rule. Violates DIP — Go arch doc
  §"Dependency inversion across
  layers" forbids rule-to-rule
  imports. Fix by extracting the
  helper into a shared package.
```

Update the audit log's `audit-from:`
field to the current `origin/main`
SHA. Then run the project's lint/fix
from the workspace root to refresh any
catalogs that list the audit log.

## Step 5: schedule

Group blockers by the structural fix
they need. One plan covers one fix,
even if it resolves several audit
entries; the plan lists each entry it
closes.

For each group:

- Create a new plan file in the
  project's plan directory using the
  next available numeric prefix.
- Title format: `arch-fix-<short-slug>`.
- Reference every audit entry the
  plan closes in the plan's "Context"
  section.
- Set the plan status to the
  project's "not started" sentinel.
- If the plan would exceed the
  project's max-file-length budget,
  split along natural seams
  (per-package, per-surface) and link
  the splits to each other in their
  "Context" sections.

For tax and nice-to-have findings,
leave them in the audit log. Mention
them in the next routine cleanup
plan.

## Step 6: tell the user

Summarize: number of blockers,
number of tax, number of
nice-to-have. List the new plan
filenames. Do not modify offending
code; the audit run is complete.

## Common skip cases

- Test fixtures (typically under
  `testdata/` or alongside variant
  packages). Their architecture is
  by design.
- Generated content between
  directive markers — review the
  directive parameters, not the
  body.
- Comments-only changes do not
  require an audit entry unless they
  document a contract that has
  changed.
- Vendored or generated code
  (`*_gen.go`, `dist/`, etc.) is
  excluded.
