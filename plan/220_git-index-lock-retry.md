---
id: 220
title: Make mdsmith's git staging tolerate transient index.lock contention
status: "🔲"
model: sonnet
summary: >-
  MDS048 and the pre-merge-commit hook both run `git add`
  without retrying, so a concurrent holder of `.git/index.lock`
  makes the staging `git add` exit 128 and aborts the merge.
  Add bounded retry-with-backoff on `index.lock` contention to
  both the in-process stage and the generated hook's staging
  loop.
depends-on: []
---
# Make mdsmith's git staging tolerate transient index.lock contention

## Goal

Stop a transient `.git/index.lock: File exists` race from aborting
a `mdsmith fix`-driven merge. Retry mdsmith's own `git add` calls
instead of failing the first time the index is briefly locked.

## Background

The merge queue repeatedly bounced a PR with:

```text
pre-merge-commit hook failed (exit 128): stats: checked=387
fixed=3 failures=3 unfixed=0 fatal: Unable to create
'.../.git/index.lock': File exists.
```

mdsmith mutates the git index in two places during the
pre-merge-commit flow, and neither retries when the index is
locked:

1. In-process, MDS048 (`git-hook-sync`) stages the regenerated
   `.gitattributes`. [`stage`](../internal/rules/githooksync/rule.go)
   calls
   [`githooks.StageGitattributes`](../internal/githooks/githooks.go),
   which runs `git add -- .gitattributes` once and records any
   error.
2. The generated hook script itself stages every fixed markdown
   file. [`BuildHookScript`](../internal/githooks/githooks.go)
   emits a `git add -- "$f"` loop under `set -e`, so one `git add`
   that fails with `index.lock: File exists` exits 128 and aborts
   the merge commit. That is the exit-128 seen in the queue.

`index.lock: File exists` means another process briefly held the
index lock. The competing holder is not identified from this
repository. It may be the merge-queue batch harness or git's own
merge bookkeeping. So the fix targets the part mdsmith owns:
tolerate a short-lived lock rather than fail on first contact.

This is independent of the engine-API work; it ships here because
the contention is what is blocking the current PR from merging.

## Tasks

1. Add a bounded retry helper for `git add` that retries only when
   git's stderr names `index.lock` (e.g. up to 5 attempts with
   short backoff), and returns the original error otherwise. Drive
   it red/green with a fake `git` that fails once then succeeds.
2. Route
   [`StageGitattributes`](../internal/githooks/githooks.go) through
   the helper, preserving its `CombinedOutput` error text so
   MDS048's staging diagnostic stays actionable.
3. Make the generated hook's staging loop in
   [`BuildHookScript`](../internal/githooks/githooks.go) retry a
   locked `git add` instead of letting `set -e` abort the merge,
   and update
   [`HookMatchesCanonical`](../internal/githooks/githooks.go) plus
   the drift fixtures so the new template is recognized as
   in-sync.
4. Update the
   [pre-merge-commit reference](../docs/reference/cli/pre-merge-commit.md)
   to note the lock-retry behavior.

## Acceptance Criteria

- [ ] A `git add` that fails once with `index.lock: File exists`
      and then succeeds is retried and reported as success by both
      the in-process stage and the generated hook loop.
- [ ] A `git add` that fails for any other reason is not retried
      and surfaces git's stderr unchanged.
- [ ] `HookMatchesCanonical` recognizes the updated hook template,
      and `pre-merge-commit status` reports no drift for a freshly
      installed hook.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## See also

- [pre-merge-commit hook](../docs/reference/cli/pre-merge-commit.md)
