---
id: 2610040911
title: Bound a timed-out recipe's leader reap and group kill
status: "🔲"
model: opus
depends-on: [2610021026]
summary: >-
  After plan 2610021026 moved `runRecipe` to `Cmd.Cancel` and
  `Cmd.WaitDelay`, a leader that survives `Process.Kill` (stuck in
  an uninterruptible call) hangs `mdsmith fix` with no bound. On
  Unix, a group that empties after its leader exited can still be
  signalled at the deadline by a reused pgid. Bound the reap and
  close the reuse window.
---
# Bound a timed-out recipe's leader reap and group kill

## Goal

A timed-out `<?build?>` recipe can never hang `mdsmith fix`, even
when its leader ignores the uncatchable kill. The deadline kill never
signals a process group whose pgid another job may have reused.

## Background

The review of PR #909 (plan
[2610021026](2610021026_recipe-exec-waitdelay.md)) found two
problems that its scope left open:

- **No bound on the leader wait.** After `WaitDelay` expires,
  `os/exec` calls `Process.Kill` and `cmd.Wait` waits for the
  leader to exit, with no limit. Before plan 2610021026,
  `runRecipe` gave up after a second `reapWait`. A leader stuck in
  a call that SIGKILL cannot interrupt (a hung NFS or FUSE read)
  now hangs `mdsmith fix`. On plan9, if the ctl write fails, only
  a note the leader can catch is sent. Plan 2610021026 chose this
  on purpose (decision 1, "Two other differences"). A test can
  only drive this through a seam around `cmd.Wait`, since no test
  can make a process that SIGKILL cannot end.
- **pgid reuse after the group empties.** When the leader exits
  first, `runRecipe` probes the group once, right after it reaps
  the leader (`pgKiller.leaderExited`). It skips the deadline kill
  when the group is already empty. A group that still had members
  then, and empties later while a setsid daemon holds the pipe,
  is still signalled by number at the deadline. By then its pgid
  may name an unrelated job.

## Tasks

1. Add a seam around `cmd.Wait` in
   [exec.go](../internal/build/exec.go). Write a failing test with
   a stub `Wait` that never returns: a timed-out run must return
   within `2 * reapWait` of the kill, with `ExitCode -1`.
2. Run `cmd.Wait` in a goroutine. Once `Cancel` has returned and
   `WaitDelay` has passed, bound it with `waitAtMost(reapWait)`.
   Document that the goroutine and the unreaped leader leak until
   the leader exits.
3. Write a failing test for a group that empties after the leader
   exits: a member exits, a setsid daemon keeps the pipe, and the
   deadline must send no SIGTERM. Fix it by probing the group on
   each drain-wait poll (or with a pidfd where the kernel has
   one). Mark the group gone at the first ESRCH.
4. Update the timeout paragraph in
   [build.md](../docs/guides/directives/build.md) to state the new
   bound.

## Acceptance Criteria

- [ ] A timed-out recipe whose leader never exits still returns
      within a documented bound.
- [ ] The deadline kill sends no signal to a recipe group that has
      emptied, wherever between the leader's exit and the
      deadline it emptied.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports
      no issues
