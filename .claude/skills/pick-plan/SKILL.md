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
  mcp__github__list_pull_requests,
  mcp__github__create_pull_request,
  mcp__github__search_pull_requests,
  AskUserQuestion
argument-hint: "[plan number]"
---

Surface the next plan to start. Read
[PLAN.md on origin/main](../../../PLAN.md). Cross-reference
existing branches and open PRs. Ask the user which plan to
pick. Create the branch and open a draft PR.

All paths below are relative to the repository root.

## When to use

Run when the user wants to start fresh work and needs to
know which plans are unclaimed. If the user already named
a specific plan number, validate it (steps 1–2) then jump
to step 5.

## Steps

### 1. Read PLAN.md from `origin/main`

```bash
git fetch --quiet origin main
```

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
places (two `121` rows, two `153`s, two `156`s) — all
completed today, but if a duplicate id ever surfaces as
`🔲`, ask which file the user means rather than guessing.

### 2. List branches and cross-reference open PRs

```bash
git branch -a --format=%(refname:short)
```

For each `🔳` and `🔲` plan, look for branches whose name
contains the plan id with a non-digit boundary on each
side — `plan-102-…`, `feature/plan-102`, `102_…`. Do
**not** treat `1020` or `2102` as a hit for plan 102.

Then pull open PRs:

Call `mcp__github__list_pull_requests` with
`owner=jeduden`, `repo=mdsmith`, `state=open`,
`perPage=100`. For each PR, derive plan ids from:

- `\bPlan[ -]?(\d+)\b` (case-insensitive) on the title
- `plan/(\d+)_` on the body or the head branch

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

### 5. Create the branch

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
times with 2s / 4s / 8s / 16s backoff (per the
[git operations policy](../../../CLAUDE.md)).

### 6. Open the draft PR

Title format: `Plan <id>: <title>` — matches the existing
convention (`Plan 200: move docs/ embed out of
internal/lsp/hover.go`).

Use `mcp__github__create_pull_request` with:

- `owner=jeduden`, `repo=mdsmith`
- `title="Plan <id>: <title>"`
- `head="plan-<id>-<slug>"`
- `base="main"`
- `draft=true`
- `body` — short:

  ```text
  Draft PR for [plan/<id>_<slug>.md](plan/<id>_<slug>.md).

  Status will move 🔲 → 🔳 in PLAN.md once the first real
  commit lands. Marking ready for review when the
  acceptance criteria are checked off.
  ```

Report the PR URL back to the user and stop. They can
run `/pr-fixup` once real changes are pushed.

## Gotchas

- **Plan-id boundary matching.** Match plan ids with a
  non-digit boundary on both sides so plan 102 doesn't
  also match a `plan-1020-…` branch. This applies both
  to the branch scan and to the PR title scan.
- **Duplicate plan ids exist.** Two `121`s, two `153`s,
  two `156`s — all completed today. If a duplicate id
  ever surfaces as `🔲`, ask which file the user means.
  The slug disambiguates.
- **`origin/main` may be stale.** Always fetch first.
  Step 1 does this, but if you re-run later parts of the
  workflow, re-fetch.
- **Empty start commit.** Step 5 makes an empty commit
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
