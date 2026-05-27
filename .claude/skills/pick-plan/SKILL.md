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
  Bash(node:*), Bash(git fetch:*), Bash(git show:*),
  Bash(git branch:*), Bash(git checkout:*),
  Bash(git push:*), Bash(git rev-parse:*),
  mcp__github__list_pull_requests,
  mcp__github__create_pull_request,
  mcp__github__search_pull_requests,
  AskUserQuestion
argument-hint: "[plan number]"
---

Surface the next plan to work on by reading
[PLAN.md on origin/main](../../../PLAN.md), cross-referencing
existing branches and open PRs, asking the user which to
start, and creating the branch plus a draft PR. The
parser is [pick-plan.mjs](./pick-plan.mjs) next to this
file — it does everything that does not require GitHub
API access.

All paths below are relative to the repository root.

## When to use

Run when the user wants to start fresh work and needs to
know which plans are unclaimed. Skip if the user already
named a specific plan number — go straight to step 5.

## Steps

### 1. Parse PLAN.md and inspect git state

```bash
node .claude/skills/pick-plan/pick-plan.mjs --json
```

The script fetches `origin/main` (best effort — offline
is fine), reads `PLAN.md` from it, and emits a JSON array
with one entry per plan:

| field      | meaning                                                               |
| ---------- | --------------------------------------------------------------------- |
| `id`       | plan number (`102`)                                                   |
| `status`   | one of `🔲` not started, `🔳` in progress, `✅` done, `⛔` superseded |
| `model`    | recommended model from the catalog (`opus`, `sonnet`, or `''`)        |
| `title`    | plan title from the catalog row                                       |
| `file`     | path under `plan/`                                                    |
| `slug`     | filename stem minus the leading number                                |
| `branches` | local/remote branches whose name contains the plan id                 |

For a quick eyeball view first:

```bash
node .claude/skills/pick-plan/pick-plan.mjs
```

### 2. Cross-reference open PRs

The driver does not talk to GitHub. Pull open PRs once
and match by plan id in title, branch, or body:

Use `mcp__github__list_pull_requests` with
`owner=jeduden`, `repo=mdsmith`, `state=open`,
`perPage=100`. For each open PR, derive a set of plan
ids by:

- regex `\bPlan[ -]?(\d+)\b` (case-insensitive) on the
  title
- regex `plan/(\d+)_` on the body or branch name

Annotate each `🔳` and `🔲` plan from step 1 with any PR
that matches its id.

### 3. Report status to the user

Print a short summary, in this order:

1. `🔳 in progress` — show plan id, title, model, the
   matching branches, and any PR number. If none of these
   exist, note that the plan is marked in-progress but no
   one has pushed yet.
2. `🔲 not started, claimed` — branch or PR exists but
   the catalog still says not-started. Flag for the user;
   do not propose starting these.
3. `🔲 available to start` — no branch, no PR. This is
   the menu.

Mention the completed count as a single line
(`106 completed`), not the list.

### 4. Ask which plan to start

Use `AskUserQuestion` with up to four of the available
plans. Pick the four to surface by:

- if the user already passed a number as an argument,
  skip the question and use that plan (step 5)
- otherwise prefer the lowest plan ids — they are usually
  the oldest open work
- include the model tag in each label so the user can
  see the recommended model at a glance

Example option label: `102 [opus] — Builder interface and
mdsmith build subcommand`.

If the user picks "Other," accept a plan number from
free-text and validate it against the JSON: must be
status `🔲`, no existing branches, no open PR. If it
fails any of those, surface why and re-ask.

### 5. Create the branch

Branch name: `plan-<id>-<slug>`, where `slug` is the
`slug` field from the JSON. Examples:

- plan 102 → `plan-102-build-subcommand`
- plan 145 → `plan-145-asdf-mise-registry-submissions`

Make sure the working tree is clean and start from
`origin/main`:

```bash
git fetch origin main
```

```bash
git checkout -b plan-<id>-<slug> origin/main
```

Create an empty commit so the branch can host a PR:

```bash
git commit --allow-empty -m "Start plan <id>: <title>"
```

```bash
git push -u origin plan-<id>-<slug>
```

If the push fails with a network error, retry up to four
times with 2s/4s/8s/16s backoff (per the
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

Report the PR URL back to the user. Stop here — the user
takes over from this point. They can `/pr-fixup` once
real changes are pushed.

## Gotchas

- **Emoji in JS regex.** The status column uses
  multi-codepoint emoji (`🔲`, `🔳`, `✅`, `⛔`). The
  driver's row regex uses alternation `(🔲|🔳|✅|⛔)`
  with the `u` flag — a character class without `u` would
  match individual surrogates and miscount everything as
  one status.
- **Plan-id boundary matching.** The branch matcher uses
  `(?:^|[^0-9])<id>(?![0-9])` so plan 102 does not also
  match a `plan-1020-…` branch. If you change the regex,
  re-check that `10` does not match `plan-102` and that
  `102` does not match `1020`.
- **Duplicate plan ids exist.** The catalog has two
  separate `121` rows and two `153`s and two `156`s, all
  completed. If you ever surface a duplicate-id row that
  is `🔲`, ask the user which file to use rather than
  guessing — the slug disambiguates.
- **`origin/main` may be stale.** The driver runs
  `git fetch --quiet origin main` first, but silently
  falls through if the fetch fails. If you suspect the
  user is offline and the catalog looks wrong, run the
  fetch yourself and rerun the driver.
- **Empty start commit.** Step 5 makes an empty commit so
  the draft PR has something to point at. Do not amend
  it — keep it as the marker that work began, and add
  real commits on top.

## Troubleshooting

- **`pick-plan: parsed zero rows from PLAN.md — catalog
  format may have changed`**: the `<?catalog?>` row
  template in `PLAN.md` was edited. Open
  [PLAN.md](../../../PLAN.md), check that rows still
  match `| id | status | model | [title](file) |`, and
  update the row regex in
  [pick-plan.mjs](./pick-plan.mjs) if so.
- **Driver hangs on `git fetch`**: the wrapper passes
  `--quiet` and swallows errors but uses the default
  network timeout. If a proxy is broken, `git fetch
  origin main --quiet` from a separate shell will show
  the same hang. Disable the fetch line in the driver if
  you need to run fully offline.
- **`AskUserQuestion` complains about option count**:
  the tool accepts 2–4 options. If only one available
  plan remains, ask a yes/no `Start plan N now?` instead.
