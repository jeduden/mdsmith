---
summary: >-
  Feasibility study for a lazy parse architecture in mdsmith's
  goldmark fork: build the CommonMark AST only when a rule actually
  navigates it, and serve the common rules from a cheap single-pass
  scanner. Grounded in what real rules read — projections like the
  code-block line set (15 rules) and link references (4) — not the
  raw node tree. Answers whether simple configs can run at
  line-scanner speed while heavy configs still get the full tree.
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
| `CollectCodeBlockLines` | AST walk      | **15** | set of line numbers inside code blocks |
| PI-block line set       | AST walk      | 5      | set of `<?…?>` directive lines         |
| `LinkReferences`        | parse context | 4      | link reference definitions             |
| `ProseRanges`           | AST walk      | 0†     | prose spans; built, no consumer yet    |
| raw `f.Lines`           | `bytes.Split` | many   | the lines themselves                   |

† `ProseRanges` is the prose projection [plan
2606022126][audit] built for a future Lines-only conversion. No rule
consumes it yet (see [Prior art](#prior-art-the-lines-only-audit)).

Fifteen rules want one fact — *which lines are code* — and reach the
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

## Prior art: the Lines-only audit

[Plan 2606022126][audit] already worked this ground, with a narrower
question. Can a standalone-`Check` rule that walks the AST be rewritten
to scan `f.Lines` instead? Its probe ran each rule twice. Once with
`f.AST` nil, once with code-block content perturbed. That reveals each
rule's true AST dependence. The result is a checked-in manifest at
`internal/integration/testdata/rule_walk_audit.json`, plus a gate that
fails any rule that regresses to `f.AST`.

Two findings carry over. The cleanly Lines-only set is empty: plans
175, 195, and 196 already moved the substring rules off the AST. And
`ProseRanges` (`internal/lint/proserange.go`) landed as a prose
projection, but no rule consumes it yet.

This study aims at a different seam. That plan rewrote rules one at a
time. It left the NodeChecker rules alone, judging their shared walk
already amortized. It never made the parse itself skippable. The lazy
parse does exactly those two things. It skips the parse when no enabled
rule needs the tree. It runs the block NodeCheckers over a skeleton. So
the manifest is the empirical oracle to validate the layer table here —
not a verdict against it.

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
  gomarklint-equivalent layer and it backs those 15 + the pure-line
  rules.
- **Layer 1 — light inline index (lazy, on first link/ref/image
  read).** A targeted byte scan for links, autolinks, images, and
  `[label]: url` definitions/uses — not the full emphasis delimiter
  algorithm. Backs the three reference rules and the bare-URL / alt-
  text rules.
- **Layer 2 — full AST (lazy, on first tree navigation).** The
  existing goldmark parse, materialized only when a rule walks the
  node graph or needs full inline structure (prose extraction,
  emphasis-as-heading, cross-file). Built once and cached on the
  `*lint.File`, exactly as today — just deferred.

The seam already exists in the code: rules call `CollectCodeBlockLines`
and `LinkReferences`, not `f.AST` directly. Re-backing those functions
to compute from Layer 0/1 when possible — and to trigger Layer 2 only
when they cannot — migrates most rules with **no rule change at all.**

## Rule-by-rule seam audit

The projection functions are the seam for rules that call them. The
other seam — the harder one — is the shared `ast.Walk` that drives the
NodeChecker rules. Auditing it returned the most useful result in this
study: **every one of the 25 NodeChecker rules is a
`KindScopedChecker`** — it declares, up front, the exact node kinds it
reacts to. The engine already dispatches by kind. So the metadata that
says which layer a rule needs *already exists in the rule*:

| Scoped kind(s)                   | NodeChecker rules                                                                                                 | Layer |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ----- |
| Heading                          | blank-line-around-headings, heading-style, no-trailing-punctuation                                                | 0     |
| FencedCodeBlock                  | blank-line-around-fenced-code, fenced-code-style, fenced-code-language, unclosed-code-block, commands-show-output | 0     |
| List / ListItem                  | blank-line-around-lists, list-marker-space, list-marker-style, ordered-list-numbering, list-indent                | 0     |
| ThematicBreak                    | horizontal-rule-style                                                                                             | 0     |
| Paragraph (trigger; body varies) | toc-directive (0), forbidden-paragraph-starts / forbidden-text (1), no-emphasis-as-heading (2)                    | 0–2   |
| HTMLBlock / RawHTML              | no-inline-html                                                                                                    | 1     |
| Link / Image / CodeSpan          | descriptive-link-text, no-space-in-link-text, no-empty-alt-text, no-space-in-code-spans                           | 1     |
| Text (URL scan)                  | no-bare-urls                                                                                                      | 1     |
| Emphasis                         | emphasis-style                                                                                                    | 2     |

Eighteen of the twenty-five react only to **block** kinds; the kind
plus line span is all those rules need. Seven react to an **inline**
kind. The general rule `emphasis-style` needs the full delimiter
algorithm (Layer 2). `no-emphasis-as-heading` reads a parsed emphasis
node too. But it asks only one bounded question: is a whole paragraph a
single emphasis span?

**Caveat: the scoped kind is the dispatch trigger, not the full data
need.** A rule scoped to `KindParagraph` still sets its own layer by
what it reads *inside* the block: `toc-directive` checks a marker
(Layer 0), the forbidden-text rules scan prose (Layer 1), and
`no-emphasis-as-heading` inspects emphasis (Layer 2). So the engine's
parse-skip gate must key on each rule's *resolved* layer, not its
trigger kind alone — a one-line per-rule annotation the migration adds
beside the existing kind scope.

A second finding sharpens the boundary: some block rules reference
*every* inline type — `blank-line-around-lists` and `list-indent` both
carry `case *ast.Text, *ast.CodeSpan, *ast.Emphasis, *ast.Link,
*ast.Image, *ast.AutoLink, *ast.RawHTML:`. That is not inline
*semantics* — it is "treat all inline as opaque, I only care about the
block." Those references disappear the moment the rule reads a block
skeleton with no inline children to enumerate. They are Layer 0, not
Layer 1.

### The one coupling to break

Block NodeCheckers receive `ast.Node` and immediately narrow it:
`heading, ok := n.(*ast.Heading)`, then read `astutil.HeadingLine` and
`f.Lines`. The type assertion is the only thing tying them to the heap
tree. Two ways to serve them without a full parse:

- **Lazy inline (smaller change).** Build *block* nodes as real
  `ast.Node` (arena-backed, cheap) but defer inline parsing per block.
  Block NodeCheckers keep working unchanged; inline-kind checkers
  trigger inline materialization. Removes the inline phase (~11%) but
  keeps the block-tree cost (~23%) — helps default configs, **not
  enough for parity.**
- **Block skeleton (the parity change).** Present each block as a flat
  `BlockSpan` (kind + line range + nesting), and adapt the block-kind
  `CheckNode`s to read kind + line from it instead of
  `n.(*ast.Heading)`. Mechanical, because they already read only kind +
  position. Removes block + inline cost — the version that can beat
  gomarklint.

`KindScopedChecker` is what makes the second option tractable: the
engine knows each enabled rule's kinds, so it can decide per run
whether any rule forces Layer 1/2, and otherwise run Layer 0 only.

### Where every rule lands

- **Layer 0 (no parse): ~33 rules.** The pure-line rules, the
  CollectCodeBlockLines consumers, the astutil section/heading rules,
  and the block-kind NodeCheckers whose body stays at block level
  (~15).
- **Layer 1 (light inline index): ~12 rules.** The link / image / URL /
  code-span / raw-HTML / reference / prose-text rules.
- **Layer 2 (full AST): the rest.** Emphasis semantics, prose
  (readability, structure, token budget, proper names), and the
  cross-file / directive rules (catalog, include, cross-file
  integrity, required-structure).

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
  Parity's block NodeChecker rules must read the Layer 0 block spans,
  not `ast.Node`. Its link, image, and reference rules read the Layer 1
  index. One parity rule resists: `no-emphasis-as-heading` reads a
  parsed `*ast.Emphasis` node, so it sits at Layer 2 today. Parity sheds
  the parse only if that rule's bounded check — is a whole paragraph one
  emphasis span? — moves to Layer 1. That one rule gates whether parity
  beats gomarklint.
- **Default / full configs.** No change, no regression. Prose,
  cross-file, and deep rules trigger Layer 2; the AST is built once
  and cached, exactly as today. Lazy parsing is a no-op here, which
  is correct — those rules need the tree.

So lazy parsing is not a universal speedup; it is a **floor
removal** for the configs whose rules never needed the tree, and a
no-op for the configs that do.

## The MDS018 holdout

`no-emphasis-as-heading` (MDS018) is the one parity rule at Layer 2. It
flags a paragraph whose only child is an emphasis node — a lone
`*emphasised line*` posing as a heading. It does not need general
emphasis resolution. It needs one bounded answer: is the whole
paragraph a single emphasis span? Two tiers clear it.

**Tier 1 — a constrained Layer 1 detector.** The check is a bounded
pattern. The paragraph content is exactly `*X*`, `_X_`, `**X**`, or
`__X__` wrapping the whole paragraph, with the CommonMark flanking
rules met at the two ends. That is far less than the delimiter
algorithm. Re-back MDS018 on it. The equivalence gate proves it
byte-identical to goldmark's lone-emphasis-child result across the
corpus and the rule fixtures. If it holds, MDS018 moves to Layer 1 and
parity is fully Layer 0/1.

**Tier 2 — per-block Layer 2, the deeper fix.** If an edge case defeats
the detector, do not concede the whole parse. Make Layer 2 per-block,
not per-file. The gate becomes "no rule forces a whole-document parse",
not "no rule needs Layer 2". MDS018 then parses inlines for only its
candidate paragraphs — the ones whose first non-space byte is `*` or
`_`. On real prose that is a handful of blocks. Parity still skips the
full-document parse.

Tier 2 is worth building even if Tier 1 holds. It future-proofs the
gate. A later rule that needs deep detail on a few blocks gets a
localized parse, not a full one.

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

- **Seam-first.** Re-back `CollectCodeBlockLines`, the PI line set,
  and `LinkReferences` to compute from Layer 0/1 with a Layer 2
  fallback. Rules that only call these need no change. (`ProseRanges`
  has no consumer to re-back yet; it is ready for the first prose-rule
  conversion.)
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
block scan) backs the 15 code-block-line consumers and the pure-line
rules with no parse; Layer 1 (a light inline index) backs the
reference and URL rules; Layer 2 (the full goldmark AST) is built
only when a rule navigates the tree. Simple and parity configs can
shed the parse entirely; heavy configs keep it, unchanged. The open
question is purely quantitative — is Layer 0 + rules + overhead under
gomarklint? — and the spike above answers it before any parser
surgery begins.

[audit]: ../../../plan/2606022126_lines-only-rule-audit.md
