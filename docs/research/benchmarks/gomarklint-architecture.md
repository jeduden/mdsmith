---
summary: >-
  How to beat gomarklint in mdsmith's parity config on benchmark 2.
  Reviews gomarklint's line-scan architecture, measures every
  optimization lever (arena, PGO, GC — all rejected with numbers),
  shows that even a free parse leaves parity above gomarklint, and
  scopes the one design that reaches the goal: a parity line-scan
  pipeline that skips the CommonMark AST entirely.
---
# gomarklint architecture and the parity gap

This page answers a single question: on benchmark 2 (the neutral
corpus — 234 Rust Book + Reference files), why does
`mdsmith-parity` run at roughly 1.8x gomarklint's wall time, and
what can close that gap?

It is a research note, not a tuning changelog. The headline
finding is architectural and does not move with micro-optimization:
**gomarklint never parses Markdown, and 27 of parity's 30 rules
force mdsmith to.**

## gomarklint in one paragraph

gomarklint (`shinagawa-web/gomarklint`, v3.2.3 — the pinned
benchmark binary) is a line scanner. `collectErrors` strips front
matter, runs `lines := strings.Split(body, "\n")` once, and hands
that `[]string` to every rule. Each rule is a plain function with
the shape:

```go
func CheckMaxLineLength(path string, lines []string, offset int, ...) []LintError
```

There is no CommonMark parse, no AST, and no node tree — ever.
Fenced-code state, heading levels, and list markers are tracked by
walking the lines with byte comparisons. A cheap prefilter,
`firstNonSpaceByte`, finds the first non-space byte of a line so
`strings.TrimSpace` only runs on lines that could match a rule.
Rules reach for `strings.HasPrefix` / `bytes.IndexByte` /
direct byte indexing rather than `regexp` in their hot paths.

Concurrency is one goroutine per file (`go func(p string)` in a
loop over the deduped path set), with a single mutex guarding result
aggregation. The external-link checker — the one rule that would
dominate — is off by default, so the default run is pure in-process
line scanning. There is no on-disk cache, which is why the benchmark
gives gomarklint no `--no-cache` flag.

That is the entire performance story: split into lines once, scan
the lines with byte ops, fan out per file. It is fast because it does
structurally less than any AST linter can.

## The measured difference

Wall-clock medians on the real 234-file neutral corpus. The
absolute numbers below are from a 4-core dev box and run higher than
the published page (different hardware); the **ratios and the
profile percentages are what transfer**, and they match the
published `gomarklint 18 ms / parity 31 ms / full 81 ms`.

| Run                                    | median  | vs gomarklint |
| -------------------------------------- | ------- | ------------- |
| gomarklint                             | ~40 ms  | 1.0x          |
| mdsmith-parity (`-c parity`)           | ~74 ms  | ~1.8x         |
| mdsmith default                        | ~105 ms | ~2.6x         |
| mdsmith repo-config (published "full") | ~170 ms | ~4.0x         |

CPU profile of the **parity** run (the apples-to-apples comparison),
share of total samples:

| Bucket                | share     | what it is                            |
| --------------------- | --------- | ------------------------------------- |
| `goldmark` parse      | ~35%      | block + inline CommonMark parse       |
| rules                 | ~36%      | the 30 enabled structural rules       |
| read + per-file setup | ~10%      | file I/O, front matter, FS, gitignore |
| merge / sort / walk   | remainder | result assembly, workspace walk       |

The single biggest cost in the parity run is the parse, and
gomarklint pays none of it. Within the parse, block parsing
(`parseBlocks` → `openBlocks`/`closeBlocks`) is ~23% and inline
parsing (`walkBlock`) is ~11%. Individual rules are each cheap —
the costliest, `atx-heading-whitespace` (MDS064), is ~7%, and most
of that is the shared code-block-line walk it happens to trigger
first, not the rule's own line scan.

## Why parity cannot skip the parse

The obvious idea — parse lazily, and skip goldmark entirely for the
cheap structural rules the way gomarklint does — does not help the
parity config. **27 of parity's 30 active rules require the AST.**
Only three are pure line scanners (`single-trailing-newline`,
`unique-frontmatter`, `no-trailing-punctuation-in-heading`).

The other 27 either implement `rule.NodeChecker` (driven by the
shared AST walk) or read `f.AST` / link references / code-block line
sets directly: `line-length` skips fenced code via the AST,
`no-bare-urls` and `link-validity` need parsed links,
`no-unused-link-definitions` and `no-undefined-reference-labels`
need goldmark's link-reference map, `list-marker-space` and
`blockquote-whitespace` walk nodes, and so on. A lazy AST is built
the moment any one of them runs — and in parity, they all run.

So the parse is not incidental overhead that better engineering can
remove. It is load-bearing for the rules parity keeps, and it is the
foundation for everything mdsmith does that gomarklint cannot:
cross-file link integrity, generated sections, schemas, rename, and
markdown-as-data. The ~35% parse cost is the architectural price of
that model.

### A note on the "full" benchmark number

The published `mdsmith = 81 ms` is partly a methodology artifact,
not a pure measure of mdsmith's defaults. The harness invokes
`mdsmith check $corpus` from the repository root, so config discovery
walks up and finds mdsmith's own `.mdsmith.yml` and applies it to the
neutral corpus — including the opt-in, Punkt-segmenter-heavy MDS024
`paragraph-structure`, which mdsmith's defaults leave **off**
precisely because the trained sentence tokenizer costs ~20% of wall
time on prose. Every other tool in the comparison runs with its own
defaults. A defaults-vs-defaults run drops mdsmith's number
substantially (~81 → ~50 ms estimated) with no code change. This is
a fairness gap in the comparison, not a regression in mdsmith;
`mdsmith-parity` already sidesteps it by selecting an explicit config.

## Goal: beat gomarklint in the parity config

The target is to make `mdsmith check -c parity` finish benchmark 2
in less wall time than gomarklint. This section records every lever
tried with its measured effect, then the one design the numbers leave
standing.

### What does not get there (measured, not guessed)

Three "free" levers were measured on the real corpus and rejected:

- **Allocation / arena.** The per-parse slab arena already absorbs
  Text, Paragraph, Segments, CodeSpan, Link, Emphasis. Extending it
  to Heading and ListItem (shipped in this PR) removes ~8.2k heap
  objects per run — they vanish from the allocation profile and the
  equivalence gate stays green — but **wall time moved within noise.**
- **PGO.** A profile-guided rebuild of `cmd/mdsmith` (the shipped
  binary is already PGO'd) left parity flat to ~1%.
- **GC tuning.** `GOGC=off` and `GOGC=800` left parity flat. The
  ~16% GC seen in the in-process bench is an artifact of running 60
  iterations back to back; the real single-shot CLI barely collects
  before it exits.

The lesson is decisive: **parity's wall time is parse + rule
computation, not allocation or GC.** Micro-optimization does not
reach gomarklint.

### Why even a free parse is not enough

Break parity's wall time into buckets (CPU profile shares):
parse ~38%, rules ~42%, per-file + walk + output ~19%. So even if the
goldmark parse cost dropped to **zero**, parity would still spend
rules + overhead ≈ 60% of its current time — roughly 48 ms against
gomarklint's ~44 ms. And the parse cannot drop to zero by tuning:
goldmark is the fastest pure-Go CommonMark parser, and mado (Rust,
which *also* parses) lands at ~29 ms next to parity's ~31 ms — every
parsing linter clusters together. gomarklint's ~18 ms is an outlier
for exactly one reason: it never builds a tree.

Conclusion: beating gomarklint requires doing what gomarklint does —
**not building the AST for the parity rule set** — *and* trimming the
rule/overhead cost below a line scanner's. Nothing short of that
clears the bar.

## A first route: the parity line-scan pipeline

This was the first design the arithmetic seemed to leave: rewrite the
parity rules as line scanners and skip goldmark for them. It was then
refined — rewriting rules forks the API in two — into the one-model
lazy parse worked out in [the lazy parse
note](lazy-parse-architecture.md). The staging below is kept for the
rule inventory and the equivalence discipline it records.

1. **Line-scan structural model.** A single-pass scanner over
   `f.Lines` that yields what the block-structure rules consume:
   per-line class (heading / fence / list / blockquote / blank /
   HTML / paragraph), code-fence line ranges, and front-matter
   bounds — gomarklint's fence-and-heading tracking, exposed on the
   `*lint.File`.
2. **Line-scan inline model.** A byte scanner for the six
   inline-dependent parity rules — links and autolinks
   (`no-bare-urls`, `link-validity`), images (`no-empty-alt-text`),
   reference definitions and uses (`no-unused-link-definitions`,
   `no-undefined-reference-labels`), and whole-paragraph emphasis
   (`no-emphasis-as-heading`) — so no parity rule needs the inline
   AST.
3. **`LineRule` capability + parse skip.** A rule interface that
   consumes the line-scan models, and an engine gate: when every
   enabled rule is line-capable (parity, and many default-style
   configs), skip `NewFileFromSourcePooled` entirely. This is where
   the ~38% parse cost actually disappears.
4. **Equivalence harness.** Diff every converted rule's output
   (line-scan vs current AST path) across the corpus and the rule
   fixtures, the same way the arena change is gated against the
   non-arena renderer — CommonMark block edge cases are the risk, and
   this is how it is contained.

**Acceptance:** `mdsmith check -c parity` beats gomarklint on
benchmark 2; every existing rule fixture passes; the line-scan vs
AST equivalence gate is green.

**Cost and risk, stated plainly.** Twenty-one block-structure rules
and six inline rules move onto the line-scan models — a large surface
with two code paths per rule to maintain, and real CommonMark
edge-case exposure (lazy continuation, setext headings, nested
fences, reference-label folding). Stage 1 (the structural scanner) is
the natural first increment and is independently testable; it yields
benchmark movement only once stage 3 lets the engine skip the parse.

## Where this PR leaves it

The arena extension shipped here is the safe down payment: a correct,
equivalence-gated allocation win that reduces GC pressure under the
file pool. It does **not** close the wall-time gap — by the
measurements above, nothing at the allocation or GC layer can. The
route to actually beating gomarklint is the lazy parse — a refinement
of the line-scan pipeline above that keeps one rule implementation
instead of forking it — scoped as staged work with a hard, measurable
acceptance bar. It is worked out in [Lazy parse architecture: build the
AST only when a rule needs it](lazy-parse-architecture.md).
