---
id: 2606171532
title: "Parity perf: fuse the per-line rules into one shared line pass"
status: "⛔"
summary: >-
  Spiked and deprioritized. The idea was to cut the rule-execution
  bucket (~47% of a parity check) by running every line-predicate rule
  in one shared pass over f.Lines. A prototype (the LineRule seam plus
  two ported rules) measured flat allocs and no wall-time win, because
  the line rules are already candidate-gated and lean — fusion saves
  only loop overhead, of which there is little. See Measured, below.
model: opus
depends-on: [2606171258]
---
# Parity perf: fuse the per-line rules into one shared line pass

## Goal

Run the line-predicate parity rules in a single pass over `f.Lines`, so
the per-rule loop, dispatch, and cache cost collapses from N scans to
one.

## Background

A CPU profile of `mdsmith check -c parity` ran over 2160 directive-free
files. The parity convention was active, so the readability, cross-file,
and directive rules are disabled. It splits the run into three buckets:

| Bucket                    | Share | What it is                       |
| ------------------------- | ----- | -------------------------------- |
| Parse + file construction | ~29%  | goldmark parse and the arena     |
| Rule execution            | ~47%  | mostly `runNonNodeCheckers`      |
| Overhead                  | ~24%  | file reads and GC write barriers |

The [parse-skip migration](2606171258_parity-layer0-parse-skip.md)
targets the first bucket. It lands parity near 1.4x gomarklint. The rule
bucket is the next lever, and it is **not** one hot rule. The cost is
spread across roughly two dozen small line rules. Each one loops
`f.Lines` on its own — trailing spaces, hard tabs, heading whitespace,
bare URLs, undefined ref labels, and peers. Each pass re-reads the same
bytes and pays its own dispatch.

## Approach

A line rule is a per-line test plus a diagnostic builder. Put those
tests behind one interface. Then drive them all from a single loop:

```go
for i, line := range f.Lines {
    for _, lr := range lineRules {
        if d, ok := lr.CheckLine(i+1, line, ctx); ok {
            diags = append(diags, d)
        }
    }
}
```

One read of each line feeds every rule, with shared per-line state (the
trimmed line, the leading-indent width, the in-code-block flag) computed
once in `ctx` rather than by each rule. The engine already groups
non-node rules; this replaces N independent `Check` loops with one.

## Tasks

1. Define a `rule.LineRule` seam: `CheckLine(num int, line []byte, ctx
   *LineContext) ([]Diagnostic, bool)`, with `LineContext` carrying the
   once-computed trimmed line, indent width, and code-block membership.
2. Port one rule (`notrailingspaces`, the profile's top line leaf) to it
   behind the existing `Check`, gated byte-identical by its unit tests.
3. Add a fused driver in the checker that runs all `LineRule`s in one
   pass; keep the per-rule `Check` path for rules not yet ported.
4. Port the remaining parity line rules one at a time, each gated.
5. Re-profile; confirm the rule bucket shrinks and parity wall time drops.

## Measured: the prototype did not win

A prototype built the `rule.LineRule` seam, a `lint.LineContext`, and a
fused driver in the checker. It ported `notrailingspaces` and
`nohardtabs` to it. The output was byte-identical: each `Check`
delegates to the same `CheckLine` the driver calls, so the rules' own
tests and the corpus equivalence gate both passed. The numbers:

- `BenchmarkCheckCorpusLarge` allocs: ~120,400/op on both main and the
  prototype — flat.
- Wall time on a 2160-file parity corpus: within noise of main, if
  anything ~4% slower with two rules fused (per-file setup not yet
  amortized).

The premise was that the rules re-scan `f.Lines` wastefully. They do
not. Each line rule already runs a **candidate gate** before any shared
lookup. For example, `atxheadingwhitespace` skips a line whose first
non-blank byte is not `#`, before it ever probes the code-block set. So
the per-line work is already minimal. Fusing the loops saves only loop
setup, which does not beat the fused driver's per-file cost.

### Conclusion

The ~47% rule bucket is not wasteful redundancy. It is the irreducible
cost of evaluating ~two dozen distinct, already-lean checks per line.
Rule fusion cannot meaningfully shrink it. The realistic ceiling for
making parity faster **without changing what it checks** is the
[parse-skip migration](2606171258_parity-layer0-parse-skip.md), near
1.4x gomarklint. Closing the rest of the gap to 1.0x would mean running
fewer rules — a change to parity's rule set, which is a product
decision, not an optimization.

## Acceptance Criteria

- [x] Prototype the seam and measure it (done; flat allocs, no win).
- [x] Record why the rule bucket does not yield to fusion (above).
- [ ] Superseded: no fusion ships. Revisit only if a future profile
      shows a large shared per-line cost the candidate gates miss.
