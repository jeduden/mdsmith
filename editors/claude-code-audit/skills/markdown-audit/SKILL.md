---
name: markdown-audit
description: >-
  Audit and fix the organization of Markdown files
  in an mdsmith repository. Catches structural
  problems the built-in rules cannot see yet:
  manually maintained indexes that should be a
  `<?catalog?>` directive, directories of similar
  files with no declared kind, repositories with no
  `.mdsmith.yml`, duplicated sections that should
  use `<?include?>`, and kinds that are missing a
  `path-pattern` or `proto.md` schema. Trigger when
  the user asks to "audit my markdown organization",
  "review how my docs are laid out", "should I use
  a kind for this directory", "this directory has a
  bunch of similar files", "I keep updating this
  index by hand", "is my .mdsmith.yml missing
  anything", or any prompt about Markdown file
  layout, kinds, schemas, or generated sections.
  Skip this skill for line-length, heading-style,
  or readability issues — those are content rules
  already enforced by `mdsmith check`.
user-invocable: true
argument-hint: "[audit | fix]"
allowed-tools: >-
  Bash(mdsmith:*),
  Bash(ls:*),
  Bash(find:*),
  Bash(grep:*),
  Bash(git ls-files:*),
  Bash(git rev-parse:*),
  Bash(test:*)
---
# Markdown audit

<?include
file: ../../../../.claude/skills/markdown-audit/SKILL.md
strip-frontmatter: "true"
?>
mdsmith ships rules for **content** (line length,
heading hygiene, readability) and rules for
**structure** (kinds, schemas, catalogs,
includes). Content rules light up out of the box.
Structure rules only fire once somebody has
declared the structure. This skill audits a
repository for *missing structure* — the patterns
mdsmith would enforce if they were wired up.

The goal is not to relint what `mdsmith check`
already lints. The goal is to surface the work
that would let `mdsmith check` lint more.

## When to run this skill

Run on any repository that uses mdsmith, or could
plausibly start using it.

- A repo with **no `.mdsmith.yml`**. The audit
  proposes an initial config plus the kinds that
  fit the current layout.
- A repo with **a `.mdsmith.yml`** that has grown
  organically. The audit catches new directories
  that drifted past existing kinds, indexes
  somebody forgot to convert, and includes that
  never got wired up.

Skip a repo that is not using mdsmith and has no
plans to. There is nothing to surface.

## Modes

Pass the mode as the skill argument.

- **`audit`** (default). Read-only. Walks the
  checks below and prints a findings report
  grouped by severity. Applies nothing.
- **`fix`**. Runs the audit, then proposes a
  concrete patch per finding and waits for
  confirmation before applying.

Default to `audit` when no argument is given. Fix
mode is opt-in because some fixes edit
`.mdsmith.yml` or move files. The user sees the
report first.

## Workflow

### 1. Locate the repository root

```bash
git rev-parse --show-toplevel
```

Run every subsequent command from the printed
path. Without `git`, use the current directory.

### 2. Detect the mdsmith CLI

Try, in order:

```bash
mdsmith version
```

```bash
go run ./cmd/mdsmith version
```

If only the second succeeds, the user is inside
the mdsmith source tree. Substitute
`go run ./cmd/mdsmith` for `mdsmith` in every
command below.

### 3. Walk the checks

Run the seven checks listed in `## Checks` in
order. Each summary names the signal, the
`<?directive?>` or `.mdsmith.yml` knob, and the
fix recipe. When a sibling `patterns.md` is
present (project install), read it once at the
start for deeper heuristics and false positives.

Do not run `mdsmith check .` from this skill.
That is the content-lint surface and runs
separately.

### 4. Emit findings

Group findings by severity (see `## Severity`).
Print all three buckets even when empty — an empty
bucket is signal that the check ran.

Audit mode stops here. Fix mode continues to step
5.

### 5. Fix loop (fix mode only)

For each finding in severity order:

1. Print the proposed patch (diff or YAML
   snippet).
2. Ask the user to confirm.
3. Apply the patch.
4. Re-run the same check on the same path to
   confirm the finding is cleared.

After every `.mdsmith.yml` edit, run
`mdsmith check .` once to surface any rule-level
regression the new config exposes. On a clean
pass, move on. Otherwise roll back the last edit
and ask the user.

The audit only *adds* structure. It does not
retune rules the user has already chosen.

## Checks

Each check has a one-line summary here.  When a
sibling `patterns.md` is present, it carries the
full signal, heuristic, false positives, and fix
recipe per check.

1. **No `.mdsmith.yml`** — the repo has no
   config; propose `mdsmith init`.
2. **Hand-maintained indexes** — a list of links
   to sibling files that should be a
   `<?catalog?>` directive.
3. **Similar files, no kind** — a directory of
   three or more `.md` files with shared front
   matter and no `kind-assignment:` entry.
4. **Duplicated content** — near-identical
   sections across files that should use
   `<?include?>`.
5. **Kind without `path-pattern`** — a kind whose
   files share a naming shape, but the body
   declares no `path-pattern:`.
6. **Kind without a schema** — a kind whose files
   share a front matter shape, but the body
   declares no `required-structure.schema:`.
7. **File placement violation** — a `.md` file
   in a directory the file-placement guide
   rejects.

## Severity

The rubric below maps every finding to one of
three buckets.

- **blocker** — the repo silently breaks
  something an agent or reader depends on.
  Examples: no `.mdsmith.yml` in a repo with many
  `.md` files; a kind declared but with no
  matching glob, so no file ever resolves to it.
- **tax** — works today; costs maintainer
  attention on every edit. Examples: a
  hand-maintained index; a duplicated section; a
  misplaced file.
- **nice-to-have** — cosmetic or aspirational.
  Examples: a kind missing `path-pattern:`; a
  kind missing a proto schema.

## Reporting format

Audit mode emits one Markdown report.

```markdown
## Audit YYYY-MM-DD

### Blockers
- `path`: pattern — fix sketch.

### Tax
- `path`: pattern — fix sketch.

### Nice-to-have
- `path`: pattern — fix sketch.

Summary: N blockers, N tax, N nice-to-have.
```

Keep the report tight. Skip the full diff in
audit mode — the diff belongs in fix mode where
the user is about to approve it.

In fix mode, after each applied fix append one
line to the report: `applied → path`.

## Notes

- Detection heuristics here are deliberately
  loose. False positives are cheaper than missed
  findings because the user reviews the report
  before any fix runs.
- When the user pushes back on a finding ("that
  is intentional"), do not add an `ignore:`
  entry for an audit-only pattern. These checks
  are not enforced by `mdsmith check`, so
  silently skipping the path next run is enough.
  Only add to `ignore:` when a *real* rule would
  otherwise fail.
- This skill ships in two places. The project
  copy at `.claude/skills/markdown-audit/` is the
  canonical source and includes a sibling
  `patterns.md` with deeper recipes; it is used by
  mdsmith contributors on this repo. The
  marketplace plugin `mdsmith-audit` mirrors the
  SKILL body via `<?include?>` so end users get
  the same workflow without the optional
  reference. Edit this file and run `mdsmith fix`
  to keep the plugin copy in sync.
<?/include?>
