---
name: pick-plan
description: >-
  Pick the next plan to start work on. Reads PLAN.md from
  origin/main, lists plans by status (completed, in
  progress, not started), cross-references open PRs and
  existing branches, asks which plan to start, then
  creates a branch and opens a draft PR. Use when asked
  to "pick a plan", "start the next plan", "what plan
  should I work on", "begin a new plan", or "open a draft
  PR for plan N".
user-invocable: true
allowed-tools: >-
  Bash(git fetch:*), Bash(git show:*), Bash(git branch:*),
  Bash(git checkout:*), Bash(git commit:*),
  Bash(git push:*), Bash(git rev-parse:*),
  Bash(git remote:*), Bash(sleep:*),
  Bash(gh pr:*), Bash(gh api:*),
  AskUserQuestion
argument-hint: "[plan number]"
---

Surface the next plan to start. Read `PLAN.md` from
`origin/main`. Cross-reference existing branches and
open PRs. Ask the user which plan to pick. Create the
branch and open a draft PR.

All paths below are relative to the repository root.

## When to use

Run when the user wants to start fresh work and needs to
know which plans are unclaimed. If the user already named
a specific plan number, validate it (steps 1–2) then jump
to step 5.

## Prerequisites

This skill assumes the clone tracks `jeduden/mdsmith`
directly and the user has push access to it. The PR
queries (steps 2 and 5) and the PR-create call
(step 7) all target that repo by name, while the push
in step 6 goes to `origin`. If `origin` is a fork,
those two ends don't line up and `gh pr create` fails
because the head branch never reaches the target.

Verify before continuing:

```bash
git remote get-url origin
```

The URL must match `jeduden/mdsmith` (`.git` suffix
optional, SSH or HTTPS both fine). If it points at a
fork, stop and tell the user. A fork-based workflow
would need `gh pr create --head <fork-owner>:<branch>`
and consistent fork-vs-upstream split throughout — out
of scope here.

## Before you run commands

Run each fenced Bash block as its own Bash call. Do
not chain commands with `&&`, do not pipe into them,
and do not prefix them with inline shell variable
assignments (`VAR=x cmd`). Allowed-tools matching
checks the command prefix, so anything that changes
the leading token (subshells, pipes, `VAR=…` prefixes)
can cause an otherwise-allowed `git` or `gh` command
to get blocked.

## Steps

### 1. Read PLAN.md from `origin/main`

```bash
git fetch --quiet --prune origin
```

This refreshes every remote-tracking ref. Step 2's
branch scan then sees newly pushed plan branches and
forgets deleted ones. `origin/main` updates in passing.

```bash
git show origin/main:PLAN.md
```

The catalog table rows look like:

```text
| 102 | 🔲     | opus   | [Builder interface and mdsmith build subcommand](plan/102_build-subcommand.md) |
```

Columns are `id | status | model | [title](file)`. Status
is one of:

| symbol | meaning     |
| ------ | ----------- |
| `🔲`   | not started |
| `🔳`   | in progress |
| `✅`   | completed   |
| `⛔`   | superseded  |

Skip the `<?catalog?>` directive markers — only parse the
table rows. The catalog has duplicate plan ids in a few
places (two `121` rows, two `153`s, two `156`s). If a
duplicate id ever surfaces as `🔲`, ask which file the
user means rather than guessing — the slug disambiguates.

### 2. List branches and cross-reference open PRs

```bash
git branch -a --format=%(refname:short)
```

For each `🔳` and `🔲` plan, look for branches whose name
contains the plan id with a non-digit boundary on each
side — `(^|[^0-9])<id>([^0-9]|$)`. Examples that
match plan 102: `plan-102-build-subcommand`,
`feature/plan-102`, `102_build-subcommand`. Examples
that must **not** match: `plan-1020-…`, `2102-…`.

Then pull open PRs:

```bash
gh pr list --repo jeduden/mdsmith --state open --limit 100 \
  --json number,title,headRefName,body
```

The repo's open-PR count is well under 100 today. If
the response ever fills the page (length == 100),
raise `--limit` and rerun. For each PR in the JSON,
derive plan ids from:

- `\bPlan[ -]?(\d+)\b` (case-insensitive) on `.title`
- `plan/(\d+)_` on `.body`
- `plan-(\d+)-` on `.headRefName` (the convention this
  skill itself creates, with the same non-digit
  boundary on the trailing side so `1020` doesn't
  match `102`)

Annotate each non-completed plan with any matching PR.

### 3. Report status to the user

Print a short summary in this order:

1. `🔳 in progress` — plan id, title, model, matching
   branches, and any PR number. If none of these exist,
   note that the plan is marked in-progress but no one
   has pushed yet.
2. `🔲 not started, claimed` — branch or PR exists but
   the catalog still says not-started. Flag these; do
   not propose starting them.
3. `🔲 available to start` — no branch, no PR. This is
   the menu.

Mention the completed count as a single line (e.g.
`106 completed`), not the list.

### 4. Ask which plan to start

If the user passed a plan number as an argument, skip the
question and validate it: must be `🔲`, no branch, no PR.

Otherwise use `AskUserQuestion` with up to four
available plans, preferring the lowest ids (usually the
oldest open work). Put the model tag in each label so
the user sees the recommendation at a glance — for
example: `102 [opus] — Builder interface and mdsmith
build subcommand`.

If the user picks "Other," accept a plan number from
free-text and run the same validation.

### 5. Check for closed PRs on this plan

Step 2 only sees open PRs. Before creating the branch,
sweep for closed PRs that referenced this plan — they
catch abandoned attempts and the rare case where a
plan is actually merged but PLAN.md is stale.

Call:

```bash
gh pr list --repo jeduden/mdsmith \
  --search '"Plan <id>:" in:title is:closed' \
  --json number,title,closedAt,mergedAt
```

The colon in the title pattern keeps `Plan 102` from
also matching `Plan 1020`.

For each hit, read `.mergedAt`:

- **non-null → merged.** Surprising. The plan is
  actually done but PLAN.md still says `🔲`. Stop.
  Tell the user the PR number, title, and merge date,
  and suggest fixing the catalog instead of starting
  fresh work.
- **null → closed unmerged.** An abandoned attempt.
  Show PR number, title, and `closedAt`. Use
  `AskUserQuestion`: "PR #N was closed unmerged on
  <date>. Start fresh anyway, or skip this plan and
  pick another?" Honor the user's choice.

If no hits, proceed.

### 6. Create the branch

Branch name: `plan-<id>-<slug>`, where `<slug>` is the
plan filename with the leading `<id>_` and trailing
`.md` stripped. Examples:

- `plan/102_build-subcommand.md` → `plan-102-build-subcommand`
- `plan/145_asdf-mise-registry-submissions.md`
  → `plan-145-asdf-mise-registry-submissions`

Start from a fresh `origin/main`:

```bash
git fetch origin main
```

```bash
git checkout -b plan-<id>-<slug> origin/main
```

Make an empty marker commit so the branch can host a PR:

```bash
git commit --allow-empty -m "Start plan <id>: <title>"
```

```bash
git push -u origin plan-<id>-<slug>
```

If the push fails with a network error, retry up to four
times with 2s / 4s / 8s / 16s exponential backoff.

### 7. Open the draft PR

Title format: `Plan <id>: <title>` — matches the existing
convention (`Plan 200: move docs/ embed out of
internal/lsp/hover.go`).

Pipe the body in via `--body-file -` so `gh` is the
only command in the call (no `$(cat ...)` subshell
that the allowlist could trip on):

```bash
gh pr create --repo jeduden/mdsmith --draft \
  --base main --head plan-<id>-<slug> \
  --title "Plan <id>: <title>" \
  --body-file - <<'EOF'
Draft PR for [plan/<id>_<slug>.md](plan/<id>_<slug>.md).

Status will move 🔲 → 🔳 in PLAN.md once the first real
commit lands. Marking ready for review when the
acceptance criteria are checked off.
EOF
```

Report the PR URL back to the user and stop. They can
run `/pr-fixup` once real changes are pushed.

## Gotchas

- **Plan-id boundary matching.** Match plan ids with a
  non-digit boundary on both sides so plan 102 doesn't
  also match a `plan-1020-…` branch. This applies both
  to the branch scan and to the PR title scan.
- **Duplicate plan ids exist.** Two `121`s, two `153`s,
  two `156`s. If a duplicate id ever surfaces as `🔲`,
  ask which file the user means. The slug disambiguates.
- **`origin/main` may be stale.** Always fetch first.
  Step 1 does this, but if you re-run later parts of the
  workflow, re-fetch.
- **Empty start commit.** Step 6 makes an empty commit
  so the draft PR has something to point at. Do not
  amend it — keep it as the marker that work began, and
  stack real commits on top.
- **Catalog format drift.** The row format above is the
  contract. If PLAN.md ever renders a different shape
  (extra column, missing model field), update this skill
  before pushing — don't paper over it with looser
  parsing.

## Troubleshooting

- **Zero plans parsed**: the `<?catalog?>` row template
  in PLAN.md was edited. Open
  [PLAN.md](../../../PLAN.md), check the catalog
  directive's `row:` line, and adjust step 1's column
  expectations to match.
- **`git fetch` hangs**: a proxy is broken. Either fix
  the network or skip the fetch and use whatever
  `origin/main` you have cached — the rest of the
  workflow still works, the catalog just might be
  stale.
- **`AskUserQuestion` complains about option count**:
  it accepts 2–4 options. If only one plan is
  available, ask a yes/no `Start plan N now?` instead.
