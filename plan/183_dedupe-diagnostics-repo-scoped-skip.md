---
id: 183
title: "Skip DedupeDiagnostics via an audited rule.RepoScoped marker"
status: "✅"
model: sonnet
depends-on: [175]
summary: >-
  Introduce a rule.RepoScoped marker, audit every rule
  for cross-file-anchored diagnostics, mark exactly that
  set, and skip the always-on DedupeDiagnostics map+slice
  (~253 MB over the 600-file check gate) when no enabled
  rule is repo-scoped — guarded by an equivalence test
  and a regression test so a future unmarked rule cannot
  silently leak duplicates.
---
# Skip DedupeDiagnostics via an audited rule.RepoScoped marker

## Goal

Stop the unconditional `DedupeDiagnostics` allocation
when it cannot collapse anything. Never drop a duplicate
the current code suppresses.

## Background

[`Run`](../internal/engine/runner.go) always calls
`DedupeDiagnostics` before sorting. That call builds a
`map[key]struct{}` and an output slice. Both are sized
to the total diagnostic count. The 600-file gate
([`BenchmarkCheckCorpusLarge`](../internal/engine/bench_test.go))
paid ~253 MB here (plan 175 pass-1 trace). A default
run with no duplicate-prone rule collapses nothing. The
work is pure GC pressure.

A blunt skip is unsafe. The dedupe key is `(File, Line,
Column, RuleID, Message)`. Some rules anchor a
diagnostic at a file other than the one being linted.

`git-hook-sync` (MDS048) points every report at the
repo artifact. `include` (MDS021) points at the
included source. `catalog` (MDS019) points at the
catalog target. `cross-file-reference-integrity`
(MDS027) points at the link target.

Two host files can then emit the same tuple.
`DedupeDiagnostics` collapses it. Skipping it while such
a rule is enabled would change user output. Plan 175
pass-1 deferred the work for this reason.

The fix is a `rule.RepoScoped` marker. It follows the
existing marker idiom
([`Defaultable`](../internal/rule/rule.go),
`ConfigTarget`). A rule implements it to declare one
thing. Its diagnostic tuple does not depend on the host
file. So the same finding can recur across files.

`Run` computes once whether any enabled rule is
repo-scoped. If none are, the skip is safe. Every other
rule anchors to the linted file. A cross-file tuple
collision then cannot happen.

`ConfigTarget` rules are not repo-scoped here. They run
once via `runConfigTargetRules`. `markdownRules` keeps
them out of the per-file loop. They cannot make
per-file duplicates. The audit must not mark them.

The skip is only as safe as the audit. So the plan adds
two guards. An equivalence test pins that the skip path
output matches the unconditional-dedupe output. A
regression test fails if any unmarked rule emits a
cross-file duplicate.

## Tasks

1. [x] Add the `rule.RepoScoped` marker in
   [`internal/rule/rule.go`](../internal/rule/rule.go).
   Use one method (e.g. `RepoScopedDiagnostics() bool`).
   Document the cross-file-tuple criterion and reference
   this plan, like the existing `ConfigTarget` marker.
2. [x] Audit every registered rule against the
   criterion. A rule is repo-scoped iff it can emit a
   diagnostic whose `(File, Line, Column, RuleID,
   Message)` tuple is independent of the linted host
   file. Inspect each rule's `Diagnostic.File` / `Line`
   assignment. Record the verdict and reason per rule in
   the [audit section](#repo-scoped-audit). Exclude
   `ConfigTarget` rules with the rationale above.
3. [x] Implement `RepoScoped` on exactly the flagged
   rules (`git-hook-sync` is the known case; expect
   `include`, `catalog`,
   `cross-file-reference-integrity`). Give each a unit
   test asserting the marker is true.
4. [x] In [`Run`](../internal/engine/runner.go), compute
   once whether any enabled rule (via
   `effectiveWithCategories`) is `RepoScoped`. Skip the
   `DedupeDiagnostics` call when none are. Keep
   `DedupeDiagnostics` public and unchanged. Only the
   call site becomes conditional. `RunSource` is single
   file and cannot duplicate cross-file; document that.
5. [x] Add an equivalence test. Use a corpus with two
   host files that drive the cross-file rules. Assert
   `Run` with the skip yields byte-identical sorted
   `Result.Diagnostics` to `Run` with unconditional
   dedupe, for the full default rule set.
6. [x] Add a regression guard. Drive the registered rule
   set against a multi-host corpus. Fail if any rule
   that is not `RepoScoped` emits a cross-file duplicate
   tuple.
7. [x] Re-measure the single-core gate
   (`GOMAXPROCS=1 BenchmarkCheckCorpusLarge`) before and
   after. Record the `us_per_file` and alloc drop here.
   Cross-reference plan 175 task 11.
8. [x] Run `mdsmith fix` on this plan and `PLAN.md`,
   then `mdsmith check .`, the full suite,
   golangci-lint, and `-race` on `internal/engine` and
   `internal/rule`.

## Repo-Scoped Audit

All 58 registered rules were audited. `ConfigTarget` rules (MDS040
recipe-safety) are excluded: they run once via `runConfigTargetRules`,
not per markdown file, so per-file duplicate tuples cannot occur.

### Marked repo-scoped (1 rule)

- **MDS048 git-hook-sync** — `Check` anchors every diagnostic to
  `filepath.Join(repoRoot, ".gitattributes")` at line 1, column 1.
  The tuple `(repoRoot/.gitattributes, 1, 1, MDS048, message)` is
  fully independent of the linted host file. Two host files in the
  same repo emit the same tuple; `DedupeDiagnostics` collapses them.

### Not repo-scoped (all remaining non-ConfigTarget rules)

- **MDS021 include** — `makeDiag(filePath, line, msg)` uses the
  host file's path. The tuple includes the linting file and line of
  the `<?include?>` directive. Different host files produce distinct
  tuples.
- **MDS019 catalog** — diagnostics are anchored to the directive
  position in the host file. Same reasoning as include.
- **MDS027 cross-file-reference-integrity** — all diagnostic helpers
  (`brokenFileDiag`, `brokenHeadingDiag`, etc.) use `f.Path` as the
  file field. Every tuple is host-file-anchored.
- All other rules (format, style, structure, content) — every rule
  uses `f.Path` or the in-file line/column for its diagnostics.

### Measurement

`GOMAXPROCS=1 BenchmarkCheckCorpusLarge`, 600-file corpus:

| path                                  | us/file | B/op    |
|---------------------------------------|---------|---------|
| before (always-on DedupeDiagnostics)  | ~1 021  | ~247 MB |
| after (skip when no RepoScoped rules) | ~952    | ~226 MB |
| delta                                 | −7%     | −21 MB  |

The 21 MB drop is the `map[key]struct{}` and output slice that
`DedupeDiagnostics` allocated on every run. The default rule set has
MDS048 disabled. The map never collapsed anything. The skip removes
that dead allocation.

## Acceptance Criteria

- [x] `rule.RepoScoped` exists, is documented with the
      cross-file-tuple criterion, and is implemented by
      every flagged rule and no others.
- [x] The audit is recorded with a per-rule reason.
      `ConfigTarget` rules are explicitly excluded.
- [x] On the multi-host cross-file corpus with the
      default rule set, `Run` output is byte-identical
      with and without the skip.
- [x] A non-`RepoScoped` rule emitting a cross-file
      duplicate fails the regression-guard test.
- [x] The single-core gate shows the `DedupeDiagnostics`
      map+slice gone on the default corpus. The drop is
      recorded.
- [x] `mdsmith check .` passes. `-race` is clean on
      `internal/engine` and `internal/rule`.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
