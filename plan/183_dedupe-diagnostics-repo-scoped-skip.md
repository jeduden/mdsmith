---
id: 183
title: "Skip DedupeDiagnostics via an audited rule.RepoScoped marker"
status: "🔲"
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

1. [ ] Add the `rule.RepoScoped` marker in
   [`internal/rule/rule.go`](../internal/rule/rule.go).
   Use one method (e.g. `RepoScopedDiagnostics() bool`).
   Document the cross-file-tuple criterion and reference
   this plan, like the existing `ConfigTarget` marker.
2. [ ] Audit every registered rule against the
   criterion. A rule is repo-scoped iff it can emit a
   diagnostic whose `(File, Line, Column, RuleID,
   Message)` tuple is independent of the linted host
   file. Inspect each rule's `Diagnostic.File` / `Line`
   assignment. Record the verdict and reason per rule in
   the [audit section](#repo-scoped-audit). Exclude
   `ConfigTarget` rules with the rationale above.
3. [ ] Implement `RepoScoped` on exactly the flagged
   rules (`git-hook-sync` is the known case; expect
   `include`, `catalog`,
   `cross-file-reference-integrity`). Give each a unit
   test asserting the marker is true.
4. [ ] In [`Run`](../internal/engine/runner.go), compute
   once whether any enabled rule (via
   `effectiveWithCategories`) is `RepoScoped`. Skip the
   `DedupeDiagnostics` call when none are. Keep
   `DedupeDiagnostics` public and unchanged. Only the
   call site becomes conditional. `RunSource` is single
   file and cannot duplicate cross-file; document that.
5. [ ] Add an equivalence test. Use a corpus with two
   host files that drive the cross-file rules. Assert
   `Run` with the skip yields byte-identical sorted
   `Result.Diagnostics` to `Run` with unconditional
   dedupe, for the full default rule set.
6. [ ] Add a regression guard. Drive the registered rule
   set against a multi-host corpus. Fail if any rule
   that is not `RepoScoped` emits a cross-file duplicate
   tuple.
7. [ ] Re-measure the single-core gate
   (`GOMAXPROCS=1 BenchmarkCheckCorpusLarge`) before and
   after. Record the `us_per_file` and alloc drop here.
   Cross-reference plan 175 task 11.
8. [ ] Run `mdsmith fix` on this plan and `PLAN.md`,
   then `mdsmith check .`, the full suite,
   golangci-lint, and `-race` on `internal/engine` and
   `internal/rule`.

## Repo-Scoped Audit

<?allow-empty-section?>

## Acceptance Criteria

- [ ] `rule.RepoScoped` exists, is documented with the
      cross-file-tuple criterion, and is implemented by
      every flagged rule and no others.
- [ ] The audit is recorded with a per-rule reason.
      `ConfigTarget` rules are explicitly excluded.
- [ ] On the multi-host cross-file corpus with the
      default rule set, `Run` output is byte-identical
      with and without the skip.
- [ ] A non-`RepoScoped` rule emitting a cross-file
      duplicate fails the regression-guard test.
- [ ] The single-core gate shows the `DedupeDiagnostics`
      map+slice gone on the default corpus. The drop is
      recorded.
- [ ] `mdsmith check .` passes. `-race` is clean on
      `internal/engine` and `internal/rule`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
