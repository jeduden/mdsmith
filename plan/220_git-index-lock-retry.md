---
id: 220
title: Harden the git-index writers against a transient index.lock
status: "✅"
model: opus
summary: >-
  The merge queue bounced when a `git add` failed on
  `.git/index.lock: File exists` during a merge. Root cause,
  confirmed from the GHA job log: the mdsmith merge driver runs
  `mdsmith fix` — including MDS048's in-process `git add` — from
  inside `git merge`, which holds the index lock, so the add
  races the merge for it. The fix drops MDS048 from the merge
  driver's rule set; a merge driver must not mutate the index.
  MDS048 still stages in the pre-merge-commit hook, which runs
  after the merge when the lock is free, and the hook's writers
  retry a transient lock as defense-in-depth.
depends-on: []
---
# Harden the git-index writers against a transient index.lock

## Goal

Stop the merge queue from bouncing on a `.git/index.lock`
failure. The root cause is the mdsmith merge driver mutating the
git index from inside `git merge`: the driver runs `mdsmith fix`,
which runs MDS048's in-process `git add`, while git holds the
index lock. Stop the merge driver from touching the index. Keep
MDS048 staging in the pre-merge-commit hook — which runs after
the merge, when the lock is free — and harden the hook's writers
against a transient lock as defense-in-depth.

## Symptom

The queue error is a `git add` failing during the merge. It
exits 128 with `fatal: Unable to create '.git/index.lock': File
exists`. The action reports it as `pre-merge-commit hook failed
(exit 128)`.

The `stats: ... fixed=3 ... unfixed=0` in the same message is
mdsmith reporting a successful `fix` (`failures`/`unfixed` are
the fix accounting, not an error). The `fatal:` is a staging
`git add` tripping over a pre-existing lock. See
[BuildHookScript](../internal/githooks/githooks.go).

## Log evidence

The failing run (PR #432 batch) settles the shape:

- It was **not** a bisect run — a single-PR batch. Nothing was
  killed.
- `git merge --no-ff --no-commit` auto-merged four
  merge-driver-managed files (`CLAUDE.md`, `AGENTS.md`,
  `PLAN.md`, `.github/copilot-instructions.md`).
- `mdsmith fix` reported success (`fixed=3 ... unfixed=0`); the
  `fatal: index.lock` and a failed `git merge --abort` came
  after.
- The lock was stale, not held by a live process: it blocked
  both the staging `git add` and the cleanup `--abort` seconds
  apart. A live process would have released it on exit.

So the lock outlived `mdsmith fix` and wedged the repo. mdsmith
is the only git-index writer in that window.

## What the cause is NOT

An earlier draft blamed the merge-queue action. It supposedly
killed a merge and left a stale lock in a shared checkout. The
run log and the action source disprove it:

- The log shows `bisect: false`. No bisection ran, nothing
  killed.
- The action has no process-killing code (`kill`, `SIGKILL`,
  `SIGTERM`, `AbortController`).
- The action itself never stages. Its flow is `git merge --no-ff
  --no-commit`, then the hook, then `git commit -m`.

## Cause

git invokes the mdsmith merge driver
([mergedriver.go](../cmd/mdsmith/mergedriver.go)) for each
conflicting `*.md` file. It runs from *inside* `git merge`, which
holds `.git/index.lock` for the whole merge.

The driver ran the fix pipeline with every rule. That includes
MDS048 (`git-hook-sync`). Its `Fix` runs an in-process `git add
-- .gitattributes` (see
[StageGitattributes](../internal/githooks/githooks.go) and its
caller in [githooksync](../internal/rules/githooksync/rule.go)).

So each driver invocation spawned a `git add` racing the parent
`git merge` for the index lock — the second index writer,
running *during* the merge, not after. With four generated files
merging at once, that is four races per merge. The hook's own
staging loop (after the merge) was never the real culprit; it
just tripped over the lock the merge-time `git add` left behind.

## Design

A merge driver runs inside `git merge` and must be a pure
content transform — it must never mutate the git index. The
primary fix enforces that; the hook-side retry is
defense-in-depth.

- The merge driver runs a rule set
  ([mergeDriverRules](../cmd/mdsmith/mergedriver.go)) that
  excludes MDS048 (`git-hook-sync`), the only index-mutating
  rule, so it performs no `git add` during `git merge`. It still
  regenerates generated sections (catalog, include, toc) to
  resolve the conflict, written through the driver's `%A`
  output.
- MDS048 still stages `.gitattributes` from its `Fix`, but only
  via the pre-merge-commit hook, which runs after `git merge`
  releases the lock. Its
  [StageGitattributes](../internal/githooks/githooks.go) call
  site retries a transient lock with bounded backoff and returns
  a clear "index locked" error if it persists.
- The hook's staging loop in
  [BuildHookScript](../internal/githooks/githooks.go) likewise
  retries a transient lock and exits with a clear "index locked"
  message on a persistent one.
- Lock-safety: no writer deletes a `.git/index.lock` it did not
  create. Retry only waits for an existing lock to clear.

## Tasks

1. [x] Exclude MDS048 from the merge driver's fix pipeline
   ([mergeDriverRules](../cmd/mdsmith/mergedriver.go)) so it does
   no `git add` while running inside `git merge`. This is the
   root-cause fix.
2. [x] Harden MDS048's
   [StageGitattributes](../internal/githooks/githooks.go) call
   site against a transient `.git/index.lock`: bounded retry with
   backoff, never delete a lock it did not create, clear "index
   locked" error on a persistent lock. Keep MDS048 staging
   `.gitattributes`.
3. [x] Harden the hook's staging loop in
   [BuildHookScript](../internal/githooks/githooks.go) the same
   way, and check `git diff`'s own exit status so a hard failure
   is not masked by the pipeline. Update
   [HookMatchesCanonical](../internal/githooks/githooks.go) and
   the golden fixtures.
4. [x] Update the
   [pre-merge-commit reference](../docs/reference/cli/pre-merge-commit.md).

## Acceptance Criteria

- [x] The merge driver performs no git-index mutation: its rule
      set excludes MDS048.
- [x] A conflicting merge of a generated file resolves through
      the driver and commits cleanly with `git-hook-sync`
      enabled, leaving no stale `.git/index.lock` and a clean
      worktree.
- [x] A transient lock that clears within the retry window is
      staged successfully, driven with a fake `git`, for both
      MDS048's `StageGitattributes` and the hook's staging loop.
- [x] A persistent lock makes the writer fail with a clear
      "index locked" message; neither writer removes a lock it
      did not create.
- [x] `HookMatchesCanonical` recognizes the updated template;
      `pre-merge-commit status` reports no drift.
- [x] Integration: after a `--no-commit` merge, run the hook,
      then commit. The merge commit captures both the regenerated
      `.gitattributes` and the fixed `*.md`. The worktree is
      clean.
- [x] All tests pass: `go test ./...`; `go tool golangci-lint
      run` reports no issues.

## Invocation model

The queue runs `git merge --no-commit`, then the hook, then a
separate `git commit`. Confirmed in
[gitops.ts](https://github.com/jeduden/merge-queue-action) and
the regression test at
[githooks_unix_test.go](../internal/githooks/githooks_unix_test.go).

Experiment shows this split is load-bearing. Under `git merge`
auto-commit, git finalizes the merge tree before the hook runs,
so the hook's staging is dropped from the commit. Under the
`--no-commit` model the same staging is captured. The fix
assumes the `--no-commit` model.

## Open questions

- Resolved. The second writer is confirmed: MDS048's in-process
  `git add`, invoked by the merge driver from inside `git merge`.
  Retry alone could not have cleared the resulting stale lock —
  it persisted for seconds, far past the ~310 ms retry budget —
  so the root-cause fix removes the index mutation from the merge
  driver.

## Open decision

Resolved 2026-05-31. The single-writer redesign for the *hook*
(remove MDS048's `git add`, make the hook the only stager) was
considered and rejected; the maintainer chose to keep MDS048
staging in the hook and harden the call sites. Separately, the
merge driver must not stage at all — the root-cause fix above.
MDS048's user-facing behavior — `fix` auto-stages
`.gitattributes` — is unchanged.

## See also

- [pre-merge-commit hook](../docs/reference/cli/pre-merge-commit.md)
