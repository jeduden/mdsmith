---
id: 2610020946
title: Kill a timed-out recipe's whole note group on plan9
status: "🔳"
model: sonnet
summary: >-
  On plan9, `internal/build` kills only a timed-out recipe's
  leader, so its children outlive the timeout. plan9 has a
  group primitive: start the recipe in its own note group
  (RFNOTEG) and write "kill" to `/proc/<pid>/notepg`. Wire
  it into a plan9-only `exec_plan9.go` so the orphan
  guarantee holds there too.
---
# Kill a timed-out recipe's whole note group on plan9

## Goal

A `<?build?>` recipe that times out on plan9 takes its
whole process tree down with it, as it already does on
Unix and on Windows with a Job Object.

## Background

Review round 3 of PR #877 (plan
[2610020725](2610020725_build-exec-js-wasm-stub.md))
found that
[exec_other.go](../internal/build/exec_other.go) treats
plan9 like js/wasm and wasip1: `killGroup` kills only the
leader. js/wasm and wasip1 cannot start a subprocess at
all, but plan9 can, and it has a group primitive. A
process started with `rfork(RFNOTEG)` leads a new note
group, and writing `kill` to `/proc/<pid>/notepg` posts
the note to every process in it. Go's
`syscall.SysProcAttr` on plan9 has `Rfork`, so
`configureProcessGroup` can set `RFNOTEG`. The bounded
post-kill wait in
[exec.go](../internal/build/exec.go) already stops
mdsmith from hanging on a survivor, so this plan is about
orphans, not hangs. It was out of scope for 2610020725,
which only had to make the package compile on wasm.

## Tasks

1. Add a failing test, tagged `plan9`, that times out a
   recipe which forks a child and asserts the child is
   gone afterwards. CI has no plan9 runner, so also
   check that `GOOS=plan9 go vet ./internal/build`
   type-checks it.
2. Split plan9 out of `exec_other.go` into
   `exec_plan9.go`. Set `SysProcAttr{Rfork:
   syscall.RFNOTEG}` in `configureProcessGroup`. In
   `afterStart`, open `/proc/<pid>/notepg` while the
   leader is alive: the kernel binds the open file to
   the note group, so a write still reaches it after
   the leader exits; also read its `noteid`. In
   `killGroup`, write `kill` to that file. The note is
   catchable, so then sweep `/proc` and write a forced
   `kill` to the `ctl` file of every process with that
   `noteid` (opening `ctl` before reading `noteid`, so
   a reused pid is never hit). Fall back to
   `cmd.Process.Kill()` if neither step reached a
   process.
3. Narrow `exec_other.go` to `!unix && !windows &&
   !plan9` and update the `runRecipe` doc comment and
   [build.md](../docs/guides/directives/build.md) so they
   say the orphan guarantee holds on plan9 too, except
   for an rc `&` job, which leads its own note group.

## Acceptance Criteria

- [ ] On plan9, a timed-out recipe leaves no process from
      its note group running.
- [x] `GOOS=plan9 go vet ./...` (tests included) passes,
      and CI still gates it.
- [x] `GOOS=js GOARCH=wasm go build ./...` still passes.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
