---
name: merge-queue
description: >-
  Enqueue a PR into the merge queue after CI is green
  and reviews are resolved.
user-invocable: true
allowed-tools: >-
  Bash(gh pr:*), Bash(gh run:*), Bash(gh api:*),
  Bash(git branch:*)
argument-hint: "[PR number]"
---

Enqueue a PR into the label-driven merge queue
(`jeduden/merge-queue-action`).

## Before you run commands

Run each fenced Bash block as its own Bash call.
Do not combine commands into one shell invocation,
and do not prefix commands with inline environment
or shell variable assignments. Allowed-tools
matching checks the command prefix, so changing
that prefix can cause an otherwise-allowed `gh`
command to be blocked.

## Steps

### 1. Identify the PR

If a PR number was passed as an argument, use it.
Otherwise detect it from the current branch:

```bash
gh pr view --json number -q '.number'
```

Note the number as `$PR`.

### 2. Verify readiness

Confirm CI is green, no review threads are
unresolved, and the latest commit has a Copilot
review before enqueuing.

Check CI:

```bash
gh pr checks "$PR" --json name,state,conclusion
```

`gh pr checks` returns `state` as `COMPLETED` or
`IN_PROGRESS`. The pass/fail verdict lives in
`conclusion`: `SUCCESS`, `FAILURE`, or `null`
while a check is still running.

Every check must have `state = COMPLETED` and
`conclusion = SUCCESS`. Stop if any check is
pending or reports a non-success conclusion.

Check for unresolved review threads:

```bash
gh api graphql -f query='
query($owner: String!, $repo: String!, $pr: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100) {
        nodes { isResolved }
      }
    }
  }
}' -f owner=OWNER -f repo=REPO -F pr="$PR" \
  -q '[.data.repository.pullRequest.reviewThreads.nodes[]
  | select(.isResolved == false)] | length'
```

Stop if the count is greater than zero. Run
`/pr-fixup` first to address the remaining
threads.

Check that Copilot reviewed the latest commit:

```bash
gh api "repos/{owner}/{repo}/pulls/$PR/reviews" \
  -q '[.[] | select(.user.login ==
  "copilot-pull-request-reviewer[bot]")]
  | last | .commit_id'
```

Compare the returned commit SHA with the PR head:

```bash
gh pr view "$PR" --json commits \
  -q '.commits[-1].oid'
```

If the two SHAs do not match, Copilot has not
reviewed the latest push. Request a review and
wait before enqueuing:

```bash
gh api --method POST \
  "repos/{owner}/{repo}/pulls/$PR/requested_reviewers" \
  -f 'reviewers[]=copilot-pull-request-reviewer[bot]'
```

### 3. Add the `queue` label

```bash
gh pr edit "$PR" --add-label queue
```

### 4. Monitor label progression

The action moves the PR through three labels:

| Label          | Meaning                           |
|----------------|-----------------------------------|
| `queue`        | PR is waiting to be picked up     |
| `queue:active` | PR is in the current batch        |
| `queue:failed` | CI failed or merge conflict found |

Check the current label:

```bash
gh pr view "$PR" --json labels \
  -q '.labels[].name'
```

Check the merge queue workflow run for the PR's
head branch (repo-wide listing would return
unrelated PRs when multiple are queued):

```bash
gh pr view "$PR" --json headRefName \
  -q '.headRefName'
```

Note the branch as `$BRANCH`, then:

```bash
gh run list --workflow merge-queue.yml \
  --branch "$BRANCH" --limit 1
```

### 5. Handle failure

If the label changes to `queue:failed`, read the
action's comment on the PR for the failure cause:

```bash
gh pr view "$PR" --comments
```

Fix the issue, push, then swap labels to
re-enter the queue:

```bash
gh pr edit "$PR" --remove-label queue:failed
```

```bash
gh pr edit "$PR" --add-label queue
```

### 6. Confirm merge

The PR is merged when `gh pr view "$PR"` shows
state `MERGED`. Report success and the merge
commit SHA:

```bash
gh pr view "$PR" --json state,mergeCommit \
  -q '.state + " " + .mergeCommit.oid'
```
