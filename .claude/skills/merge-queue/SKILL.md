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

## Steps

### 1. Identify the PR

If a PR number was passed as an argument, use it.
Otherwise detect it from the current branch:

```bash
gh pr view --json number -q '.number'
```

Note the number as `$PR`.

### 2. Verify readiness

Confirm CI is green and no review threads are
unresolved before enqueuing:

```bash
gh pr checks "$PR" --json name,state
```

All checks must show `SUCCESS`. If any are
`FAILURE` or `PENDING`, stop and report the
blockers instead of enqueuing.

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

Check the merge queue workflow run:

```bash
gh run list --workflow merge-queue.yml \
  --limit 1
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
