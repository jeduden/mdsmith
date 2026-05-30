---
id: 220
title: Make the pre-merge-commit hook the single git-index writer
status: "🔲"
model: opus
summary: >-
  The merge queue bounces when the pre-merge-commit hook's `git
  add` fails on `.git/index.lock: File exists`. The action source
  rules out a concurrent or killed process, so the trigger is still
  under investigation. Regardless, MDS048 does an in-process `git
  add` that no linter should. Make the hook the single stager and
  remove MDS048's index mutation.
depends-on: []
---
# Make the pre-merge-commit hook the single git-index writer

## Goal

Stop the merge queue from bouncing on a `.git/index.lock`
failure. Remove the in-process `git add` that `mdsmith fix` should
never perform. Leave one git-index writer: the hook.

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

## What the cause is NOT

An earlier draft blamed the merge-queue action. It supposedly
killed merge attempts and left a stale lock in a shared checkout.
Reading the action source at the pinned `v0.7.8` disproves that:

- The action has no process-killing code. There is no `kill`,
  `SIGKILL`, `SIGTERM`, or `AbortController` anywhere in it.
- Bisect re-dispatches the workflow. Each attempt is a fresh run
  with a fresh `actions/checkout`. No checkout is shared across
  attempts.
- Batch PRs merge in a sequential `await` loop. The workflow's
  `concurrency` group keeps runs from overlapping.
- The action never stages. Its flow is `git merge --no-ff
  --no-commit`, then invoke the hook, then `git commit -m`.

So no concurrent or killed process holds the lock.

## Leading hypothesis

The lock must arise inside one sequential run. The prime suspect
is mdsmith's own index writer. MDS048 (`git-hook-sync`) runs an
in-process `git add -- .gitattributes` during `mdsmith fix`. See
[StageGitattributes](../internal/githooks/githooks.go) and its
caller in [githooksync](../internal/rules/githooksync/rule.go).

This is unconfirmed as the trigger. The exact cause needs the
failed run logs or a local repro (see Open questions). The fix
below stands on design grounds regardless of the trigger.

## Design

Mutating the git index is orchestration, not linting. Today two
mdsmith writers touch the index during a merge: the hook's staging
loop and MDS048's in-process `git add`. The fix leaves one writer.

- The pre-merge-commit hook is the only mdsmith component that
  stages. It runs after `git merge --no-commit`, while git has
  paused, so the index is free in the normal path.
- MDS048 writes `.gitattributes` as a pure content transform. It
  performs no `git add`. The hook stages `.gitattributes`.

## Tasks

1. In one atomic change, remove the in-process `git add` from
   MDS048 ([StageGitattributes](../internal/githooks/githooks.go)
   and its caller in
   [githooksync](../internal/rules/githooksync/rule.go)). Extend
   the hook's staging loop in
   [BuildHookScript](../internal/githooks/githooks.go) to add
   `.gitattributes` beside `*.md` / `*.markdown`. Ship both halves
   together. Experiment shows that removing the in-process `git
   add` while the hook stages only `*.md` drops the regenerated
   `.gitattributes` from the merge commit. Update
   [HookMatchesCanonical](../internal/githooks/githooks.go) and the
   golden fixtures. Adjust MDS048's diagnostics and tests.
2. Harden the hook's staging loop against a transient lock. Use
   bounded retry with backoff. Never delete a lock it did not
   create. Exit with a clear "index locked" message on a
   persistent lock. This is defensive; no concurrent writer is
   known.
3. Update the
   [pre-merge-commit reference](../docs/reference/cli/pre-merge-commit.md).

## Acceptance Criteria

- [ ] `mdsmith fix` performs no in-process git index mutation:
      MDS048 does no `git add`.
- [ ] The hook stages `.gitattributes` beside `*.md` /
      `*.markdown`, so a merge that regenerates `.gitattributes`
      still captures it.
- [ ] A transient lock that clears within the retry window is
      staged successfully, driven with a fake `git`.
- [ ] A persistent lock makes the hook exit with a clear "index
      locked" message. The hook never removes a lock it did not
      create.
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

- The exact lock trigger is unconfirmed. The source rules out a
  concurrent or killed process. Confirm whether MDS048's
  in-process `git add` creates the lock, or whether an earlier job
  step does. The failed run logs (`actions/runs/26667938542`,
  `26677854302`) would settle it.

## Open decision

Task 1 changes MDS048's behavior. Today its `fix` auto-stages
`.gitattributes`. Afterward a manual `mdsmith fix` leaves it
modified but unstaged, and the hook stages it during merges. The
alternative keeps staging in MDS048 and only hardens the call
site. The single-writer design is recommended. Confirm before
implementing.

## See also

- [pre-merge-commit hook](../docs/reference/cli/pre-merge-commit.md)
