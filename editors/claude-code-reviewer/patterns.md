---
title: Detection patterns
summary: >-
  Detection heuristics, false positives, and fix
  recipes for the three config-level checks the
  `markdown-reviewer` agent runs that no built-in
  rule backs.
---
# Detection patterns

These three checks have no backing rule, so their
signal and fix live here rather than in a rule
README. Each section covers one numbered
config-level check from the sibling agent
workflow: the signal, the heuristic, false
positives to skip, the severity bucket, and the
fix recipe.

The other checks come from built-in rules. Load
their signal and fix from `mdsmith help patterns
-f json`. The agent workflow step 3 has the call.

## Check 1 — No `.mdsmith.yml`

Signal: `.mdsmith.yml` does not exist at the repo
root (`test -f .mdsmith.yml`).

False positives: a repo with one or two top-level
Markdown files and nothing else. Skip with a
one-line note.

Severity: blocker when the repo has five or more
`.md` files. Nice-to-have otherwise.

Fix: run `mdsmith init` to create a default
config. Then walk the remaining checks to
populate kinds, overrides, and assignments.

## Check 2 — Similar files, no kind

Signal: a directory holds three or more `.md`
files with a similar front matter shape. The
files share the same required keys. No
`kind-assignment:` entry covers the directory.

Heuristic: list directories with three or more
`.md` files
(`git ls-files '*.md' | xargs -n1 dirname | sort | uniq -c`).
For each candidate, sample every file's front
matter. The directory is a kind candidate when 70%
of files share the same key set, and no glob in
`.mdsmith.yml` already covers it.

False positives:

- A directory of one-off Markdown without shared
  front matter (research spikes, ad-hoc notes).
- A directory already covered by a glob in
  `kind-assignment:`, even when the kind body is
  empty. The assignment alone counts.
- Bad fixtures inside `internal/rules/*/bad/`,
  intentionally malformed and under `ignore:`.

Severity: tax. Drop to nice-to-have when the
shared shape is small.

Fix: add an entry to `kinds:` plus a matching
`kind-assignment:` glob.

```yaml
kinds:
  runbook:
    rules:
      max-file-length:
        max: 400

kind-assignment:
  - glob: ["runbooks/*.md"]
    kinds: [runbook]
```

Start with the settings the files actually share
right now. Do not over-engineer. After applying,
run `mdsmith kinds resolve runbooks/example.md` to
confirm the file picks up the new kind.

## Check 3 — Kind without `path-pattern`

Signal: a declared kind whose files share an
obvious naming convention, but the kind body has
no `path-pattern:`.

Heuristic: read `kinds:` and `kind-assignment:`
from `.mdsmith.yml`. For each kind, walk the files
that resolve to it. The kind is a candidate when
80% of those files share a regex-shaped filename.
Examples: `\d+_[a-z-]+\.md`, `RFC-\d{4}\.md`.

False positives: a kind with intentionally
heterogeneous membership — a "doc" kind covering
README, index, and topic pages qualifies.

Severity: nice-to-have.

Fix: add `path-pattern:` to the kind body.

```yaml
kinds:
  plan:
    path-pattern: "plan/[0-9][0-9]*_*.md"
```

Glob syntax has no "exactly digits" class, so the
pattern is an approximation. For tighter
constraints, combine `path-pattern:` with a
`<?require filename:?>` directive on the kind's
schema. Re-run `mdsmith check .` to confirm no
existing file fails the new pattern.
