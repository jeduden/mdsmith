---
name: pick-plan
description: >-
  Pick the next plan to start work on. Reads PLAN.md
  from origin/main, cross-references open PRs and
  existing branches, reads each non-completed plan's
  `depends-on:` frontmatter, filters out plans whose
  dependencies are not yet completed, splits the rest
  into "structurally important" (other plans depend
  on them) and "quick wins" (smaller-model plans),
  presents the top 4, then creates a branch, opens a
  draft PR, and dispatches an implementation Agent at
  the model the plan declares. Use when asked to
  "pick a plan", "start the next plan", "what plan
  should I work on", "begin a new plan", or "open a
  draft PR for plan N".
user-invocable: true
allowed-tools: >-
  Bash(git fetch:*), Bash(git show:*), Bash(git branch:*),
  Bash(git ls-tree:*), Bash(git checkout:*),
  Bash(git commit:*), Bash(git push:*),
  Bash(git rev-parse:*), Bash(git remote:*),
  Bash(sleep:*), Bash(gh pr:*), Bash(gh api:*),
  AskUserQuestion, Agent
argument-hint: "[plan number]"
---

Surface the next plan. Filter out the ones blocked
by unfinished dependencies. Rank the rest into
"structurally important" and "quick wins" and let
the user pick. Create the branch and open a draft
PR. Hand the implementation off to an Agent at the
model the plan declares.

All paths below are relative to the repository root.

## When to use

Run when the user wants to start fresh work. If the
user already named a specific plan number, do not
skip ahead — step 5 still runs that id through the
full status/branch/PR/dependency validation before
the workflow moves on to step 6.

## Prerequisites

This skill assumes the clone tracks `jeduden/mdsmith`
directly and the user has push access to it. The PR
queries (steps 2 and 6) and the PR-create call
(step 8) all target that repo by name, while the push
in step 7 goes to `origin`. If `origin` is a fork,
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

### 3. Read `depends-on:` for every non-✅ plan

For each plan with status `🔲` or `🔳`, read the file
from `origin/main` and extract its `depends-on:`
frontmatter list. Treat a missing key as `[]`.

```bash
git show origin/main:plan/<id>_<slug>.md
```

Run one `git show` per non-completed plan. The line
is `depends-on: [146, 147]` (inline YAML); parse the
integers between the brackets.

Build two maps from the results. Key plan records by
filename, not by id — the catalog has duplicate plan
ids and two plans sharing an id must not collapse to
one node:

- `deps[filename]` → list of plan ids this plan
  needs.
- `dependents[id]` → count of plan-files whose
  `deps` includes this id. A single id may resolve
  to multiple filenames when duplicates exist; the
  dependents count sums every plan-file that points
  at that id, regardless of which duplicate they
  meant. That's the safe direction — a dep is "met"
  only when every plan sharing that id is `✅`.

Skip the duplicate-id problem by reading by filename
(you already have it from PLAN.md). If a plan's
`depends-on:` references an id that isn't in
PLAN.md at all (typo, deleted plan), treat that dep
as unmet so the plan stays blocked until someone
fixes the typo — don't silently ignore it.

### 4. Report status to the user

Print a short summary in this order:

1. `🔳 in progress` — plan id, title, model, matching
   branches, and any PR number. If none of these
   exist, note that the plan is marked in-progress
   but no one has pushed yet.
2. `🔲 not started, claimed` — branch or PR exists
   but the catalog still says not-started. Flag
   these; do not propose starting them.
3. `🔲 blocked by deps` — list the plan id and which
   unmet deps it's waiting on (use the ids from
   `deps`, mark each unmet dep with its status,
   e.g. `103 (🔲)`). Do not propose starting these.
4. `🔲 available to start` — no branch, no PR, all
   deps `✅`. This is the menu source.

Mention the completed count as a single line
(e.g. `106 completed`), not the list.

### 5. Categorize available plans and ask which to start

If the user passed a plan number as an argument, skip
the question and validate it: must be `🔲`, no branch,
no PR, and every `depends-on:` id must be `✅`. If
the dep check fails, list the unmet deps and stop —
don't auto-pick a different plan.

Otherwise sort the available set into two buckets.
A plan can qualify for both. Prefer **Structurally
important** when a plan fits both:

- **Structurally important** — `dependents[id] > 0`.
  Other open plans are waiting on this one. Sort
  by `dependents` desc, then id asc.
- **Quick wins** — smaller-model plans, sorted by the
  plan's frontmatter `model:` field with `haiku`
  before `sonnet` before `opus` before empty/missing,
  then id asc. The model tag in PLAN.md is the
  proxy for plan size; an empty field means
  "unranked", treat it as the largest.

Take the top 2 from each bucket for the menu. If one
bucket has fewer than 2, fill from the other while
keeping each bucket's internal order. Cap the
`options` array you pass to `AskUserQuestion` at 4 —
that is the tool's hard limit. The tool then renders
an "Other" choice automatically on top of those, so
the user always has a free-text fallback; you do not
add it to `options` yourself.

Use `AskUserQuestion` with each option labeled like:

- `🏗 102 [opus] — Builder interface and mdsmith build subcommand`
- `⚡ 207 [sonnet] — LSP fix preview`

(`🏗` for structurally important, `⚡` for quick wins.
Include the model tag so the user sees which model
the implementation Agent will spawn at.)

If the user picks the auto-rendered "Other," accept a
plan number from free-text and run the same dep-check
validation.

If after filtering there's only one available plan,
ask a yes/no `Start plan N now?` instead of a menu.

If the filtered set is empty, report that every
unblocked plan is either claimed or waiting on deps
and stop.

### 6. Sanity-check for closed PRs on this plan

Step 2 only sees open PRs. Before creating the
branch, sweep for closed PRs that referenced this
plan — they catch abandoned attempts and the rare
case where a plan is actually merged but PLAN.md is
stale.

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

### 7. Create the branch

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

Make an empty marker commit so the branch can host a
PR:

```bash
git commit --allow-empty -m "Start plan <id>: <title>"
```

```bash
git push -u origin plan-<id>-<slug>
```

If the push fails with a network error, retry up to
four times with 2s / 4s / 8s / 16s exponential
backoff.

### 8. Open the draft PR

Title format: `Plan <id>: <title>` — matches the
existing convention (`Plan 200: move docs/ embed out
of internal/lsp/hover.go`).

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

Capture the PR number from the output for step 10.

### 9. Suggest a session name

No programmatic API can set a Claude Code web
session's title from inside the session. Tell the
user, in one line, to run the slash command. That
makes the session findable in the picker and the
mobile app:

```text
Suggested session name. Run:
/rename plan-<id>-<slug>
```

Print it once, don't loop. The user runs it (or
doesn't); either way, proceed to step 10.

### 10. Dispatch the implementation Agent at the plan's model

Read the plan's frontmatter `model:` field. Map it to
the Agent tool's `model` parameter:

| frontmatter | Agent `model`                                     |
| ----------- | ------------------------------------------------- |
| `haiku`     | `haiku`                                           |
| `sonnet`    | `sonnet`                                          |
| `opus`      | `opus`                                            |
| empty/`""`  | `opus` (default — biggest model for unsized work) |

Spawn one `Agent` call with `subagent_type: "claude"`,
`model` set per the table above, and a self-contained
prompt. The Agent has no view of this conversation,
so the prompt must include: plan id, plan title,
plan file path, branch name, draft PR number, and
the implementation discipline. Use this shape:

```text
Implement plan <id>: <title>.

Branch `plan-<id>-<slug>` is already checked out from
`origin/main` with one empty marker commit, and draft
PR #<pr-num> on jeduden/mdsmith is open against it.
Do not recreate the branch or PR.

Read plan/<id>_<slug>.md. Work through its `## Tasks`
section using red/green TDD: write a failing test,
make it pass, commit. Keep commits small and focused
on one change.

Plan maintenance: in your first real commit, edit the
plan's frontmatter `status:` from `🔲` to `🔳` and
run `mdsmith fix PLAN.md` so the catalog reflects it.
Check off tasks and acceptance criteria as you verify
them — update the plan file as part of implementation,
not as a follow-up.

Before every commit, run `mdsmith check .` (must pass)
and `go test ./...` for any package you touched. Do
not modify `.mdsmith.yml`. Push after each commit
with `git push -u origin plan-<id>-<slug>`.

When every acceptance criterion is checked off, move
the frontmatter `status:` from `🔳` to `✅`, run
`mdsmith fix PLAN.md`, push, then return a summary:
final commit count, files touched, and any deviations
from the plan that you made along the way. Don't
mark the PR ready for review — leave that to the
user.

If you hit a design decision the plan doesn't pin
down, stop and surface it in your return message
instead of guessing.
```

Set `description` to `Implement plan <id>` (short,
since this is what shows in the Agent's status line).

When the Agent returns, relay its summary to the user
in two or three sentences plus the PR URL from step 8.
The user can then run `/pr-fixup` to babysit CI and
review comments.

## Gotchas

- **Plan-id boundary matching.** Match plan ids with
  a non-digit boundary on both sides so plan 102
  doesn't also match a `plan-1020-…` branch. This
  applies to the branch scan, the PR title scan, and
  the `depends-on:` parse.
- **Duplicate plan ids exist.** Two `121`s, two
  `153`s, two `156`s. If a duplicate id ever surfaces
  as `🔲`, ask which file the user means. The slug
  disambiguates. Always key the dependency graph by
  filename, not just id, so a duplicate doesn't
  collapse two plans into one node.
- **`origin/main` may be stale.** Always fetch first.
  Step 1 does this, but if you re-run later parts of
  the workflow, re-fetch.
- **Empty start commit.** Step 7 makes an empty
  commit so the draft PR has something to point at.
  Do not amend it — keep it as the marker that work
  began, and stack real commits on top.
- **Catalog format drift.** The row format above is
  the contract. If PLAN.md ever renders a different
  shape (extra column, missing model field), update
  this skill before pushing — don't paper over it
  with looser parsing.
- **Empty `model:`.** Treat an empty or missing
  frontmatter `model:` as `opus` when dispatching the
  Agent. Don't ask the user to pick a model — the
  whole point is that the plan declares it.
- **Session rename is advisory.** The skill can't
  call `/rename` for the user; the slash command
  only runs when typed into the prompt bar. Step 9
  prints the suggestion and moves on.

## Troubleshooting

- **Zero plans parsed**: the `<?catalog?>` row
  template in PLAN.md was edited. Open
  [PLAN.md](../../../PLAN.md), check the catalog
  directive's `row:` line, and adjust step 1's
  column expectations to match.
- **`git fetch` hangs**: a proxy is broken. Either
  fix the network or skip the fetch and use whatever
  `origin/main` you have cached — the rest of the
  workflow still works, the catalog just might be
  stale.
- **Every available plan is blocked**: step 5 reports
  the empty set. Look at the `🔲 blocked by deps`
  list from step 4 — the unblocking work is whichever
  in-progress (`🔳`) plan is named in those `deps`
  entries. Suggest the user finish that one first
  rather than picking around it.
- **Agent dispatched at the wrong model**: re-read
  the plan's frontmatter directly with
  `git show origin/main:plan/<id>_<slug>.md` and
  confirm the `model:` field. If the plan says one
  model and the Agent ran at another, the mapping
  table in step 10 was misapplied — fix the call,
  don't try to "switch model" inside the running
  Agent (it can't).
- **`AskUserQuestion` complains about option count**:
  it accepts 2–4 options. Step 5 covers the
  single-option and zero-option cases; check that
  path if you're hitting the error.
