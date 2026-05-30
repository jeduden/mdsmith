---
id: 220
title: Make merge-time staging robust against index-lock contention
status: "🔲"
model: opus
summary: >-
  The merge queue bounces with `git add` failing on
  `.git/index.lock: File exists`. The lock is stale, left by a
  killed merge attempt, so retry is futile. Fix the root cause in
  the merge-queue orchestration, make the pre-merge-commit hook
  lock-aware without stealing locks, and stop MDS048's `fix` from
  mutating the git index in-process.
depends-on: []
---
# Make merge-time staging robust against index-lock contention

## Goal

Stop the merge queue from bouncing on a `.git/index.lock`
failure. Fix the stale-lock root cause. Make every mdsmith piece
behave correctly around a locked index.

## Root cause

The queue error is a `git add` that hits an existing
`.git/index.lock`. Local experiments establish the mechanism:

- A plain `git merge` runs the `pre-merge-commit` hook with **no**
  lock held; the hook's `git add` succeeds. The hook is correct
  for local developers, and the failure does not reproduce on a
  normal merge.
- A **stale** `.git/index.lock` reproduces the exact queue error.
  Three retries all fail identically. Only removing the lock
  clears it. So retry-with-backoff cannot fix a stale lock.

The lock is stale, not live. The merge-queue action bisects a
failing batch by killing merge attempts. A process killed
mid-index-write leaves `.git/index.lock` behind in the shared
checkout. Later attempts in that run then fail deterministically.
The action labels this "transient" and retries, but nothing
clears the lock, so it never converges.

The `stats: ... fixed=3 ... unfixed=0` in the error is mdsmith
reporting success. The `fatal:` comes afterward, from the hook's
`set -e` staging loop. See
[BuildHookScript](../internal/githooks/githooks.go).

## Design

Mutating the git index is an orchestration job, not a linter job.
Each piece gets one responsibility:

- **merge-queue-action** owns process lifecycle, so it owns lock
  cleanup. It must isolate each merge attempt and never leave a
  stale lock.
- **The pre-merge-commit hook** stages fixed files into the merge
  commit. It runs while git has paused, so the index is free in
  the normal path. It must tolerate a brief live lock, never steal
  a lock it does not own, and fail clearly on a persistent lock.
- **MDS048** should write `.gitattributes` as a pure content
  transform. Staging it belongs to the hook, not to a `fix` rule.

## Tasks

1. **(merge-queue-action — separate repo, owner-implemented.)**
   Run each merge attempt in a throwaway `git worktree` or clone,
   and remove `.git/index.lock` when an attempt is killed or times
   out, so a killed bisection step cannot poison sibling attempts.
2. Make the hook's staging loop in
   [BuildHookScript](../internal/githooks/githooks.go) lock-aware:
   bounded retry with backoff for a transient lock, never delete a
   lock it did not create, and exit with a clear "index locked"
   message on a persistent lock. Update
   [HookMatchesCanonical](../internal/githooks/githooks.go) and
   the golden fixtures.
3. Remove the in-process `git add` from MDS048
   ([StageGitattributes](../internal/githooks/githooks.go) and its
   caller in
   [githooksync](../internal/rules/githooksync/rule.go)) so
   `mdsmith fix` creates no `.git/index.lock`. Extend the hook to
   stage `.gitattributes` alongside `*.md`. Adjust MDS048's
   diagnostics and tests.
4. Update the
   [pre-merge-commit reference](../docs/reference/cli/pre-merge-commit.md).

## Acceptance Criteria

- [ ] `mdsmith fix` creates no `.git/index.lock`: MDS048 performs
      no in-process git index mutation.
- [ ] A transient lock that clears within the retry window is
      staged successfully (driven with a fake `git`).
- [ ] A persistent lock makes the hook exit with a clear "index
      locked" message, and the hook never removes a lock it did
      not create.
- [ ] `HookMatchesCanonical` recognizes the updated template, and
      `pre-merge-commit status` reports no drift.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## Open decision

Task 3 changes MDS048's behavior: today its `fix` auto-stages
`.gitattributes`; afterward a manual `mdsmith fix` leaves it
modified but unstaged, and the hook stages it during merges. The
alternative is to keep staging in MDS048 and only harden both
call sites. The centralized design is recommended; confirm before
implementing.

## See also

- [pre-merge-commit hook](../docs/reference/cli/pre-merge-commit.md)
