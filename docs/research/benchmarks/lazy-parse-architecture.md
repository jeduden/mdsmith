---
summary: >-
  Feasibility study for a lazy parse architecture in mdsmith's
  goldmark fork: build the CommonMark AST only when a rule actually
  navigates it, and serve the common rules from a cheap single-pass
  scanner. Grounded in what real rules read — projections like the
  code-block line set (16 rules), link references (4), and prose
  ranges (2) — not the raw node tree. Answers whether simple configs
  can run at line-scanner speed while heavy configs still get the
  full tree.
---
# Lazy parse architecture: build the AST only when a rule needs it

The benchmark study ([gomarklint architecture and the parity
gap](gomarklint-architecture.md)) showed that a separate line-scan
code path beside the AST means two implementations per rule and a
forked API — the wrong design. This note studies the alternative:
**one parser whose output is cheap by default and materializes the
heavy CommonMark tree only when a rule reads it.**

The question to answer: can a lazy parse run simple scenarios at
line-scanner speed and pay the AST cost *only* when a rule actually
needs the tree?

Short answer from the evidence below: **yes — because rules almost
never read the raw tree. They read a handful of cheap projections,
and those projections are the natural lazy boundary.**

## What rules actually read

Auditing every rule's access to the parsed file is the whole game. A
rule that "uses the AST" usually does not navigate the node graph —
it calls one derived projection and consumes a flat result. The
projections, by how many rules depend on each:

| Projection              | Backed by     | Rules  | What it is                             |
| ----------------------- | ------------- | ------ | -------------------------------------- |
| `CollectCodeBlockLines` | AST walk      | **16** | set of line numbers inside code blocks |
| PI-block line set       | AST walk      | 6      | set of `<?…?>` directive lines         |
| `LinkReferences`        | parse context | 4      | link reference definitions             |
| `ProseRanges`           | AST walk      | 2      | source spans of prose text             |
| raw `f.Lines`           | `bytes.Split` | many   | the lines themselves                   |

Sixteen rules want one fact — *which lines are code* — and reach the
full CommonMark parse to get it. That fact is exactly what a
fence-tracking line scanner produces in one pass (it is most of what
gomarklint computes). The AST is doing enormously more than these
rules consume.

### Three rules, three depths

**Simple — `line-length` (MDS001), the most-run rule.** Its core is
a byte-length scan over `f.Lines`. It touches the "AST" only through
`CollectCodeBlockLines` (to skip code) and `collectHeadingLines` (for
an optional per-heading limit); its *table* exclusion is already a
pure byte scan (`isTableLineStart`, no AST). So MDS001 needs three
line classifications — code, heading, table — and nothing else. No
node graph, no inline tree. A skeleton scanner satisfies it
completely.

**Block — the NodeChecker rules (`blank-line-around-*`,
`list-marker-space`, `list-indent`, `fenced-code-*`).** These ride a
single shared `ast.Walk`, but they inspect *block* nodes — headings,
lists, list items, fenced code — and their line positions. They need
block structure, not inline detail. A block skeleton that records
each block's kind and line span serves them; the pointer-linked tree
and the inline phase are surplus.

**Heavy — `cross-file-reference-integrity` (MDS027),
`paragraph-readability` (MDS023), the link/reference rules.** These
genuinely need depth: MDS027 resolves link targets across files
(reference map + filesystem + other files' parses); MDS023 extracts
prose text (inline projection) and scores it; `no-undefined-
reference-labels` needs the normalized reference map (Unicode case
folding included). These are the rules that *should* pay for a richer
representation — and only these.

## The lazy boundary is the projection layer

Today the data flow is eager and one-directional:

```text
source --> goldmark parse (block + inline, full tree) --> projections --> rules
```

Every run pays the full parse so the projections can be derived,
even when the only enabled rule is `no-trailing-spaces`.

The lazy design inverts the dependency: projections become the
primary product of a cheap scanner, and the full tree is one more
projection — built on demand.

```text
source --> cheap scan ──> line classes / code+PI line sets / fm bounds
                      └──> light link & reference index
                      └──> [lazy] full goldmark AST  <-- only on first
                                                          tree navigation
```

- **Layer 0 — line model (always).** `f.Lines` plus a one-pass block
  state machine: per-line class (heading / fence / list / blockquote
  / blank / HTML / paragraph), the code-block and PI line sets, and
  front-matter bounds. Allocation-lean, no node tree. This is the
  gomarklint-equivalent layer and it backs the 16 + the pure-line
  rules.
- **Layer 1 — light inline index (lazy, on first link/ref/image
  read).** A targeted byte scan for links, autolinks, images, and
  `[label]: url` definitions/uses — not the full emphasis delimiter
  algorithm. Backs the four reference rules and the bare-URL / alt-
  text rules.
- **Layer 2 — full AST (lazy, on first tree navigation).** The
  existing goldmark parse, materialized only when a rule walks the
  node graph or needs full inline structure (prose extraction,
  emphasis-as-heading, cross-file). Built once and cached on the
  `*lint.File`, exactly as today — just deferred.

The seam already exists in the code: rules call `CollectCodeBlockLines`,
`LinkReferences`, `ProseRanges`, not `f.AST` directly. Re-backing
those functions to compute from Layer 0/1 when possible — and to
trigger Layer 2 only when they cannot — migrates most rules with **no
rule change at all.**

## Feasibility verdict, by scenario

The honest answer to "fast for simple, expensive only when needed"
is per active rule set, because the engine builds whatever the
enabled rules force:

- **Simple configs (line + projection rules only).** Big win, low
  effort. With Layer 0 backing `CollectCodeBlockLines` and the line
  rules, a config like "trailing spaces + line length + hard tabs +
  heading style" never builds the AST. This is gomarklint-class
  speed, and it covers a large share of real-world `.mdsmith.yml`
  setups.
- **Parity (the benchmark target).** The prize, and the real work.
  Parity's block NodeChecker rules must consume the Layer 0 block
  spans instead of `ast.Node`, and its six inline rules must consume
  the Layer 1 index. Once every parity rule reads Layer 0/1, the
  engine skips Layer 2 and parity stops parsing — the only path to
  beating gomarklint.
- **Default / full configs.** No change, no regression. Prose,
  cross-file, and deep rules trigger Layer 2; the AST is built once
  and cached, exactly as today. Lazy parsing is a no-op here, which
  is correct — those rules need the tree.

So lazy parsing is not a universal speedup; it is a **floor
removal** for the configs whose rules never needed the tree, and a
no-op for the configs that do.

## The honest performance bar

Two facts must hold for this to beat gomarklint on parity, and a
spike should confirm both before the parser is touched for real:

1. **Layer 0 must be near-line-scan cheap.** If the block state
   machine allocates a tree or rescans per rule, it is just goldmark
   again. Target: one pass, flat spans, no per-node heap object.
2. **Rules + overhead alone must fit under gomarklint.** Even with a
   free parse, parity's rules + walk + output were ~60% of its wall
   time (~48 ms vs gomarklint's ~44 ms in the benchmark note). Layer
   0 removes the parse, but if rules + overhead still exceed
   gomarklint, the rule and per-file pipeline need trimming too. The
   spike below measures exactly this.

**Spike (next concrete step):** add a parse mode that runs Layer 0
only (block scan, no inline, no tree) and time `Layer 0 + the parity
structural rules + overhead` against gomarklint on the neutral
corpus. The number decides whether the rearchitecture clears the bar
or whether rule/overhead trimming must ride alongside it. It is a
contained measurement on the fork — no rule migration required to get
the number.

## Migration path and risk

- **Seam-first.** Re-back `CollectCodeBlockLines` / PI lines /
  `LinkReferences` / `ProseRanges` to compute from Layer 0/1 with a
  Layer 2 fallback. Rules that only call these need no change.
- **NodeChecker adapter.** The shared `ast.Walk` dispatch is the
  migration's hardest seam: block NodeCheckers need a skeleton-node
  view, inline NodeCheckers need Layer 1. This is where most of the
  work and risk sit.
- **Equivalence gate, non-negotiable.** Every projection and rule
  served from Layer 0/1 must produce byte-identical output to the
  Layer 2 path, diffed across the corpus and every rule fixture —
  the same discipline that gates the arena change against the
  non-arena renderer. CommonMark block edge cases (lazy
  continuation, setext, nested fences) and reference-label
  normalization are where divergence hides.
- **One model, not two.** Rules consume the projection API; whether
  it is served from a scan or the tree is invisible to them. There is
  no second rule implementation and no forked API — the objection
  that killed the two-path design.

## Conclusion

A lazy parse is feasible and well-shaped for mdsmith because the
rules already read projections, not the raw tree. Layer 0 (a cheap
block scan) backs the 16 code-block-line consumers and the pure-line
rules with no parse; Layer 1 (a light inline index) backs the
reference and URL rules; Layer 2 (the full goldmark AST) is built
only when a rule navigates the tree. Simple and parity configs can
shed the parse entirely; heavy configs keep it, unchanged. The open
question is purely quantitative — is Layer 0 + rules + overhead under
gomarklint? — and the spike above answers it before any parser
surgery begins.
