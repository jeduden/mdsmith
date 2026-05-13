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

Walk this list during an audit run. The
audit is a survey; do not edit offending
code in the same run. For each finding,
append an entry to the audit log with:

- File(s) involved.
- Principle violated (one of: SRP, OCP,
  LSP, ISP, DIP) or layering rule
  ("dependency direction", "boundary
  contract", etc.).
- Severity: `blocker`, `tax`, or
  `nice-to-have`.
- Suggested fix (one sentence).

The audit log lives at
`docs/development/architecture-audit.md`
(create it on the first run; see "Initial
file" below).

## Initial file

If `docs/development/architecture-audit.md`
does not exist, create it with this
shape:

```markdown
---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
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

Pick a starting SHA one month back unless
the user supplies a different baseline.

## Step 1: refresh the checkpoint

Read the last audit SHA from the audit log's
front matter (`audit-from:`). Call it
`<from-sha>` below; paste the literal SHA
into each command rather than capturing it
into a shell variable (the allowed-tools
allowlist matches on the command's leading
token, so a `VAR=… cmd` prefix or `$(…)`
capture would block the call).

Fetch:

```bash
git fetch origin main
```

List the files touched since the checkpoint
— replace `<from-sha>` with the actual SHA
before running:

```bash
git log --name-only --pretty=format: <from-sha>..origin/main
```

Keep the file list in your notes as the
"touched set" for the rest of this audit;
do not assign it to a shell variable.

## Step 2: Go layering checks

For each Go file in the touched set:

- Does it import only from lower layers?
  Cross-check against the layering map in
  the [architecture hub](index.md).
- Is its package still answering one
  question? If the package has grown three
  new files in unrelated areas, flag it
  for split (SRP).
- Are new `rule.Rule` implementations
  using only `rule.*` interfaces and not
  reaching into `internal/engine`? (DIP)
- Does any new function in
  `internal/engine` only serve one rule?
  If so, flag it for relocation. (SRP)
- Are new interfaces minimal? An interface
  with one implementor is a smell unless
  it sits on a boundary. (ISP)
- Do any new error types add fields that
  callers ignore? Flag for simplification.
- Are there new packages named `util`,
  `common`, `helpers`, or `lib`? Always a
  blocker (SRP).

## Step 3: TypeScript layering checks

For each `.ts` file in the touched set
under `editors/vscode/`:

- Does a command import another command?
  Flag (DIP).
- Did `extension.ts` gain logic? Flag — it
  should only call `wiring.ts` (SRP).
- Did `binary.ts` grow non-binary
  concerns? Flag (SRP).
- Are new tests new modules or new
  assertions on existing ones? New
  command modules without tests are
  blockers.
- Did `package.json` `contributes` gain a
  field with no owning module? Blocker.
- Did any command call `child_process`
  directly outside `binary.ts` or
  `commands/runner.ts`? Blocker (DIP).

## Step 4: cross-system contract checks

- Did the `.mdsmith.yml` schema gain a
  field? If yes, is it documented under
  `docs/reference/`? Is it backward
  compatible? If not, blocker.
- Did CLI flags change? Run
  `mdsmith help` and diff against the
  reference docs. Any renamed or removed
  flags are blockers without a
  deprecation window.
- Did the LSP server gain a capability?
  Is there a fixture test? Did the VS
  Code client opt into it (if expected
  to)?
- Did `npm/`, `python/`, or the plugin
  manifest gain logic that should live in
  the binary? Flag.
- Did a generated section marker syntax
  change in any way that breaks parsing
  of older markers? Blocker.

## Step 5: severity rubric

- **blocker** — violates dependency
  direction, breaks a published contract,
  or makes a future refactor materially
  harder. Schedule a fix before the next
  release. File a plan under `plan/` with
  a back-link to the audit entry.
- **tax** — works today but adds friction
  (e.g. a function in the wrong package,
  a rule with one-off settings). Schedule
  inside the next plan cycle; leave the
  entry in the audit log until picked up.
- **nice-to-have** — cosmetic (naming,
  small duplication, doc tightening).
  Batch into a cleanup commit when
  convenient.

## Step 6: record findings

Append to the audit log under a new
`## Audit YYYY-MM-DD (range: …)` section.
For each severity, list bullets:

```markdown
### blockers

- `internal/engine/foo.go`: imports
  `internal/rules/mds001-...`. Violates
  DIP — engine must not depend on rule
  packages. Fix by routing through
  `rule.Rule`.
```

Update the `audit-from:` field to the
current `origin/main` SHA. Then run
`mdsmith fix` on the audit log so any
catalog stays current.

## Step 7: schedule

Group blockers by the structural fix they
need. One plan covers one fix, even if it
resolves several audit entries; the plan
lists each entry it closes out.

For each group:

- Create a new plan file under `plan/`
  using the next available numeric
  prefix. Title format:
  `arch-fix-<short-slug>`. Reference every
  audit entry the plan closes in the
  plan's "Context" section.
- Set the plan `status` to `🔲`.
- If the plan would exceed
  `mdsmith check`'s `max-file-length`,
  split it along the natural seams of the
  fix (per-package, per-surface) and have
  the split plans link to each other in
  the "Context" section.

For each tax and nice-to-have:

- Leave in the audit log. Mention in the
  next routine cleanup plan.

## Step 8: tell the user

Summarize: number of blockers, number of
tax, number of nice-to-have. List the new
plan filenames. Do not modify offending
code; the audit run is complete.

## Common skip cases

- Files under `testdata/` and
  `internal/rules/MDS###-<rule-name>/{good,bad}/`
  are fixtures; their architecture is by
  design.
- Generated section bodies (between
  `<?directive?>` markers) are
  auto-produced; review the directive
  parameters, not the body.
- Comments-only changes do not require an
  audit entry unless they document a
  contract that has changed.
- Vendored or generated Go code
  (`*_gen.go`, code under `internal/…/gen`)
  is excluded.
