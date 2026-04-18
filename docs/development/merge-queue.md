---
title: Merge Queue
summary: Label-driven merge queue workflow using jeduden/merge-queue-action.
---
# Merge Queue

PRs merge via `jeduden/merge-queue-action`, not the
GitHub merge button. Add the `queue` label after CI
is green and reviews are resolved:

```bash
gh pr edit --add-label queue
```

**How it works**: the action collects all PRs labeled
`queue` (oldest first), merges up to `batch_size` (5)
into a temporary `merge-queue/batch-*` branch
server-side, then dispatches the CI workflow against
that branch. If CI passes, `main` fast-forwards to
the batch head and the batch branch is deleted.

**Label state machine**: three labels track
progression through the queue:

| Label          | Meaning                           |
|----------------|-----------------------------------|
| `queue`        | PR is waiting to be picked up     |
| `queue:active` | PR is in the current batch        |
| `queue:failed` | CI failed or merge conflict found |

On failure the action adds `queue:failed` and posts
a comment explaining the cause. Fix the issue, push,
then replace the label to re-enter the queue:

```bash
gh pr edit --remove-label queue:failed
gh pr edit --add-label queue
```

**Bisection**: when a multi-PR batch fails, the
action recursively bisects the batch until the
failing PR is isolated. Single-PR failures are
reported immediately.

**Eligibility**: only same-repo PRs targeting `main`.
Fork PRs cannot enter the queue.

**Manual dispatch**: maintainers can trigger the
queue workflow via `workflow_dispatch`. Pass PR
numbers in `batch_prs` or enable `bisect` mode to
debug a failing batch.

The [PR fixup workflow](pr-fixup-workflow.md) covers
getting a PR to merge-ready state. Use the
`/merge-queue` skill to enqueue.
