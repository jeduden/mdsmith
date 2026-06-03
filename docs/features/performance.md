---
title: "Fast on every run"
summary: >-
  A single static Go binary, no runtime to start. The workspace walk
  runs in parallel, embeds are linted once, and `check` is built for
  the hot path — roughly 4x faster than Node markdownlint, with a CI
  gate against regression.
icon: zap
link: "/reference/cli/check/"
weight: 6
group: "One engine, every surface"
---
# Fast on every run

Speed is a feature, not an afterthought.

**No runtime cold-start.** mdsmith ships as one static Go binary.
There is no Node, Python, or JVM process to launch before work
begins. Process start time is measured in single-digit milliseconds.

**Parallel workspace walk.** Files fan out across all available
cores. Files pulled in by `<?include?>` and `<?catalog?>` are
linted once regardless of how many host documents reference them —
a large doc set does not re-scan the same prose repeatedly.

**Hot-path design.** `mdsmith check` shares the rule engine with
the LSP server and the fixer. Editor feedback, `git commit` hooks,
and CI all run the same optimised core — no per-surface overhead.

Concretely: a small working set is near-instant. An 18-file
`docs/features` check is ~50 ms. A full check of this
repository — ~720 tracked Markdown files, full rule suite,
cross-file link resolution — is ~1.3 s on commodity hardware.
That still beats a Node markdownlint over the same files by
roughly 4x. A CI gate (`check-bench`, modelled on the LSP
latency gate) fails the build if a 60- or 600-file synthetic
check regresses past its budget. The cross-tool numbers and method
are in the
[benchmark research doc](../research/benchmarks/README.md).

See the [`check`](../reference/cli/check.md) reference for flags
and exit codes.
