---
id: 137
title: "`mdsmith fix --dry-run`"
status: "✅"
model: sonnet
summary: >-
  Add `--dry-run` to `mdsmith fix` so agents and CI
  scripts can preview the file changes before
  writing. Emits the same diagnostics as `check` plus
  a per-file count of fixes that would apply.
---
# `mdsmith fix --dry-run`

## Goal

Let a user run `mdsmith fix --dry-run` and see
exactly which files would change, which rules
would fire, and how many violations each file
carries. No bytes are written. The exit code
matches what `fix` would have returned, so a CI
script can gate on dry-run output.

## Background

Plan **C-6** in the
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md)
calls this out as a quick ergonomic win. The
trigger is an agent loop or CI run that wants a
safety net before writing.

Today the only preview is `mdsmith check`, which
reports diagnostics but does not say which would
be auto-fixed. A user who wants to know "what
will `fix` change?" has to diff the worktree
afterward.

The fix engine already produces a fixed-content
buffer per file before writing. Gating the write
behind a flag is small.

## Non-Goals

- Diff output. The flag prints which rules would
  fire and how many; a textual diff per file is
  out of scope. Users who want a diff can run
  `mdsmith fix` against a copy.
- Partial application. The flag is all-or-nothing
  per run. No "fix MDS001 but preview MDS006".
- A separate subcommand. `--dry-run` on `fix`
  matches the convention other CLIs use.

## Design

### Flag behavior

```bash
mdsmith fix --dry-run docs/
```

Output, one line per file with at least one fix:

```text
docs/api.md: would fix 3 violations (MDS001 ×2, MDS006)
docs/index.md: would regenerate <?catalog?> body
```

Files with no fixes do not appear. A trailing
summary matches the existing `fix` summary line:

```text
stats: checked=12 fixed=0 failures=0 unfixed=4 would-fix=8
```

The dry-run summary keeps the existing
`checked=` / `fixed=` / `failures=` / `unfixed=`
fields for machine-parsability. `fixed=0` is
always literal-zero on a dry run, since nothing
was written. `would-fix=N` is additive — the
count of violations a real run would have
auto-fixed. A consumer that only watches
`fixed=` therefore sees 0 (no surprise apply);
a consumer that watches `would-fix=` sees the
preview count.

### JSON output

`--format json` emits one record per file:

```json
{"path":"docs/api.md","would_fix":3,
 "rules":["MDS001","MDS006"],"diagnostics":[]}
```

The `diagnostics` array carries the same shape
`check --format json` returns today.

### Exit code

The exit code is the same `fix` would have
returned without `--dry-run`:

- `0` — every diagnostic is fixable; a real run
  would leave the worktree clean.
- non-zero — at least one unfixable diagnostic
  remains.

This lets a CI step run dry-run as a gate on
"fix can clean this PR".

### Compatibility

`--dry-run` is additive. Default behavior is
unchanged. The new line in the summary
(`would-fix=N`) is suppressed on a real run.

## Tasks

1. Add `--dry-run` to the `fix` subcommand flag
   set in
   [`runFix` in `cmd/mdsmith/main.go`](../cmd/mdsmith/main.go).
2. Gate the write step in the fix pipeline:
   build the fixed buffer as today, but on
   `--dry-run` skip the write and record the
   would-fix count per rule.
3. Update the per-file output formatter to emit
   "would fix N violations" lines and the
   trailing `would-fix=N` summary stat.
4. Extend the JSON output struct with
   `would_fix` and `rules` fields when
   `--dry-run` is set.
5. Document the flag in
   [`docs/reference/cli/fix.md`](../docs/reference/cli/fix.md)
   with one worked example.
6. Tests:

  - dry-run reports the same fix count a real
    run would apply (regression compares both),
  - dry-run leaves the worktree byte-identical,
  - dry-run exit code matches the real-run exit
    code on the same input,
  - JSON output exposes `would_fix` and
    `rules`.

## Acceptance Criteria

- [x] `mdsmith fix --dry-run` writes nothing to
      disk (regression test asserts every
      candidate file is byte-identical after
      the run).
- [x] The per-file output names each rule that
      would fire and how many times.
- [x] The summary line includes
      `would-fix=N`. The existing
      `checked=` / `fixed=` / `failures=` /
      `unfixed=` fields are present on the
      same line; `fixed=` reads `0` since
      nothing was written.
- [x] `--format json` exposes `would_fix` and
      `rules` per file.
- [x] Exit code matches the real-run exit code
      on identical input.
- [x] [`docs/reference/cli/fix.md`](../docs/reference/cli/fix.md)
      documents the flag with an example.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
