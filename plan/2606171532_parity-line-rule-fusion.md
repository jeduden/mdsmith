---
id: 2606171532
title: "Parity perf: fuse the per-line rules into one shared line pass"
status: "🔲"
summary: >-
  Cut the rule-execution bucket (measured ~47% of a parity check, the
  largest after the parse) by running every line-predicate parity rule
  in one shared pass over f.Lines instead of each rule re-scanning the
  file. This is the second lever, after the parse-skip migration, on the
  path to gomarklint's wall time on benchmark 2.
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

## Acceptance Criteria

- [ ] The fused driver runs every ported line rule in one `f.Lines` pass.
- [ ] Each ported rule is byte-identical to its pre-fusion output
      (unit tests plus the corpus equivalence gate).
- [ ] A re-profile shows the rule-execution bucket smaller than ~47%.
- [ ] `go test ./...` passes.
