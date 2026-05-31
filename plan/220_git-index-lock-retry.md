---
id: 220
title: Harden the git-index writers against a transient index.lock
status: "🔳"
model: opus
summary: >-
  The merge queue bounces when the pre-merge-commit hook's `git
  add` fails on `.git/index.lock: File exists`. The action source
  rules out a concurrent or killed process, so the trigger is still
  under investigation. mdsmith has two git-index writers during a
  merge: MDS048's in-process `git add` and the hook's staging loop.
  Harden both with bounded retry/backoff and a clear "index locked"
  failure on a persistent lock, never deleting a lock it did not
  create.
depends-on: []
---
# Harden the git-index writers against a transient index.lock

## Goal

Stop the merge queue from bouncing on a `.git/index.lock`
failure. mdsmith has two git-index writers during a merge:
MDS048's in-process `git add` and the pre-merge-commit hook's
staging loop. Harden both against a transient lock instead of
collapsing to one writer, so MDS048's user-facing behavior is
unchanged.

## Symptom

The queue error is the pre-merge-commit hook's `git add` failing.
It exits 128 with `fatal: Unable to create '.git/index.lock':
File exists`. The action reports it as `pre-merge-commit hook
failed (exit 128)`.

The `stats: ... fixed=3 ... unfixed=0` in the same message is
mdsmith reporting a successful `fix`. The `fatal:` comes
afterward, from the hook's `set -e` staging loop. See
[BuildHookScript](../internal/githooks/githooks.go).

A local experiment reproduces the error. A `git add` that hits a
pre-existing `.git/index.lock` fails with this exact message. It
fails the same way on retry. It recovers only when the lock is
removed.

## Log evidence

The failed run (`actions/runs/26677854302`) settles the cause:

- It was **not** a bisect run. The inputs were `bisect: false`,
  `batch_prs: 423` — a single-PR batch. Nothing was killed.
- A normal content merge ran (auto-merging `CLAUDE.md`,
  `AGENTS.md`, `PLAN.md`, `.github/copilot-instructions.md`), then
  the hook ran.
- `mdsmith fix` reported success: `checked=387 fixed=3 unfixed=0`.
  The `fatal: index.lock` came after.
- The lock was stale, not held by a live process. The action's own
  cleanup then failed on it too: `git merge --abort` exited 128
  and `git reset` could not reset the index. A live process would
  have released its lock on exit.

So the lock outlived `mdsmith fix` and wedged the repo. mdsmith is
the only git-index writer in that window.

## What the cause is NOT

An earlier draft blamed the merge-queue action. It supposedly
killed a merge attempt and left a stale lock in a shared checkout.
The run log and the action source at `v0.7.8` both disprove it:

- The log shows `bisect: false`. No bisection ran, nothing killed.
- The action has no process-killing code (`kill`, `SIGKILL`,
  `SIGTERM`, `AbortController`).
- Bisect, when it does run, re-dispatches a fresh workflow with a
  fresh `actions/checkout`. No checkout is shared.
- The action never stages. Its flow is `git merge --no-ff
  --no-commit`, then the hook, then `git commit -m`.

## Cause

The lock arises inside mdsmith's own hook execution. mdsmith has
two git-index writers during a merge. MDS048 (`git-hook-sync`)
runs an in-process `git add -- .gitattributes` during `mdsmith
fix`; see
[StageGitattributes](../internal/githooks/githooks.go) and its
caller in [githooksync](../internal/rules/githooksync/rule.go).
The hook's own staging loop then runs `git add`.

The log cannot order the two `git add` calls: the action captures
all hook stderr as one block. It does not need to. The fix hardens
both writers against a transient `index.lock` so a brief lock no
longer bounces the queue, whichever writer hit it.

## Design

Two mdsmith writers touch the index during a merge: MDS048's
in-process `git add -- .gitattributes` and the hook's staging
loop. Both are kept; both are hardened against a transient lock.

- MDS048 still stages `.gitattributes` from its `Fix` so the
  rule's user-facing behavior is unchanged. Its
  [StageGitattributes](../internal/githooks/githooks.go) call site
  retries a `git add` that fails on a `.git/index.lock` with
  bounded backoff, and returns a clear "index locked" error if the
  lock persists.
- The pre-merge-commit hook still stages the markdown files
  `mdsmith fix` touched (and `.gitattributes` is already staged by
  MDS048, which the hook runs via `mdsmith fix .`). Its staging
  loop in [BuildHookScript](../internal/githooks/githooks.go)
  retries a `git add` that fails on a lock with bounded backoff,
  and exits with a clear "index locked" message if the lock
  persists.
- Lock-safety: neither writer deletes a `.git/index.lock` it did
  not create. The retry only waits for an existing lock to clear;
  on a persistent lock it fails loudly rather than forcing the
  lock away.

## Tasks

1. Harden MDS048's
   [StageGitattributes](../internal/githooks/githooks.go) call
   site against a transient `.git/index.lock`. Use bounded retry
   with backoff. Never delete a lock it did not create. Return a
   clear "index locked" error on a persistent lock. Keep MDS048
   staging `.gitattributes` (no behavior change for the rule).
2. Harden the hook's staging loop in
   [BuildHookScript](../internal/githooks/githooks.go) against a
   transient lock. Use bounded retry with backoff. Never delete a
   lock it did not create. Exit with a clear "index locked"
   message on a persistent lock. Update
   [HookMatchesCanonical](../internal/githooks/githooks.go) and the
   golden fixtures.
3. Update the
   [pre-merge-commit reference](../docs/reference/cli/pre-merge-commit.md).

## Acceptance Criteria

- [ ] A transient lock that clears within the retry window is
      staged successfully, driven with a fake `git`. This holds
      for both MDS048's `StageGitattributes` and the hook's
      staging loop.
- [ ] A persistent lock makes the writer fail with a clear "index
      locked" message (MDS048 returns the error; the hook exits
      non-zero). Neither writer removes a lock it did not create.
- [ ] `HookMatchesCanonical` recognizes the updated template.
      `pre-merge-commit status` reports no drift.
- [ ] Integration: after a `--no-commit` merge, run the hook, then
      commit. The merge commit captures both the regenerated
      `.gitattributes` and the fixed `*.md`. The worktree is clean.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## Invocation model

The queue runs `git merge --no-commit`, then the hook, then a
separate `git commit`. Confirmed in
[gitops.ts](https://github.com/jeduden/merge-queue-action) and the
regression test at
[githooks_unix_test.go](../internal/githooks/githooks_unix_test.go).

Experiment shows this split is load-bearing. Under `git merge`
auto-commit, git finalizes the merge tree before the hook runs.
The hook's staging is then dropped from the commit. Under the
`--no-commit` model, the same staging is captured. So the fix
assumes the `--no-commit` model. If the action ever switches to
auto-commit, staging breaks even today.

## Open questions

- The exact sub-mechanism is unconfirmed: whether MDS048's
  in-process `git add` leaves a stale lock, or collides with the
  hook's staging loop. The log localizes the bug to mdsmith but
  cannot order the two `git add` calls. A local repro that installs
  the real hook and runs the merge would pin it. The fix does not
  depend on the answer.

## Open decision

Resolved 2026-05-31. The single-writer redesign (remove MDS048's
in-process `git add`, make the hook the only stager) was
considered and rejected. The maintainer chose the less invasive
alternative: keep MDS048 staging `.gitattributes` and only harden
the two call sites against a transient `index.lock`. MDS048's
user-facing behavior — `fix` auto-stages `.gitattributes` — is
unchanged.

## See also

- [pre-merge-commit hook](../docs/reference/cli/pre-merge-commit.md)
