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

### Spike results (measured)

[Plan 2606141901][spike-plan] built the measurement. A block-only parse
mode (goldmark's block phase with the inline walk and AST transformers
suppressed — `parser.WithBlockOnly`, reached through the engine's
default-off `Runner.BlockOnlyParse` flag) stands in for Layer 0. It is
an **upper bound** on Layer 0's parse cost: goldmark's block phase still
builds a block node tree, where a real flat scan would be cheaper. Two
harnesses ran over the pinned neutral corpus (234 Rust Book + Reference
files, 2.2 MiB, 7419 parity diagnostics) on a 4-core dev box, against
gomarklint v3.2.3:

- an in-process serial decomposition
  (`internal/engine/spike_blockparse_test.go`, gated on
  `MDSMITH_SPIKE_CORPUS`) that times each phase of the lint pipeline
  with GC pinned and output excluded; and
- a CLI hyperfine run (`--warmup 3 --runs 15 -N`) that times the real
  `mdsmith check` wall clock, with `MDSMITH_SPIKE_BLOCK_ONLY=1`
  selecting the block-only mode.

**Lint-pipeline CPU decomposition** (serial min, share of the 88 ms
full-parity lint CPU — parse, rules, and per-file work, but *not* output
rendering):

| Phase                       | Share |
| --------------------------- | ----- |
| read + front-matter + split | ~3%   |
| block parse                 | ~16%  |
| inline parse                | ~21%  |
| rules + merge + per-file    | ~60%  |

So the goldmark parse is ~37% of the lint CPU and rules + overhead is
~63% — the same split the [gomarklint study](gomarklint-architecture.md)
estimated (parse ~38%, rules + overhead ~61%), now measured rather than
profiled.

**CLI wall time** (hyperfine medians; ratios are what transfer across
hardware, per the benchmark note):

| Run                         | wall    | vs gomarklint |
| --------------------------- | ------- | ------------- |
| gomarklint                  | ~31 ms  | 1.0x          |
| mdsmith-parity (full parse) | ~59 ms  | ~1.87x        |
| mdsmith-parity (block-only) | ~53 ms  | ~1.70x        |
| mdsmith default             | ~134 ms | ~4.27x        |

**Go / no-go: Layer 0 alone does NOT clear the bar — rule and overhead
trimming must ride alongside it.** Three readings converge on it:

- Suppressing the inline parse in the *real CLI* cut parity's wall time
  only ~9% (59 → 53 ms) and left it at ~1.70x gomarklint — and that
  figure is optimistic, because under block-only the inline rules see no
  nodes and silently no-op (and emit fewer diagnostics, so even output
  shrinks). A genuine Layer-0/1 pipeline would run those rules over a
  light inline index, costing *more* than 53 ms.
- The parse is ~37% of the lint CPU but the lint CPU is only part of the
  wall clock (output rendering and process start are parse-independent
  and, on this diagnostic-heavy corpus, large — parity's user CPU is
  ~134 ms against a ~59 ms wall). Scaling the parse share to the wall
  clock puts it near ~14 ms of the ~59 ms. Even a *free* parse leaves
  parity at ~45 ms — still ~1.4x gomarklint.
- The parse-free floor (read + rules + overhead) is ~63% of the lint
  CPU, ~37 ms of equivalent wall — already above gomarklint's ~31 ms.
  This is the honest bar's point 2, now measured: rules + overhead alone
  exceed gomarklint.

The lazy parse is still worth building for the reason the rest of this
note argues — it removes the floor for the *simple* configs whose rules
never needed the tree (the largest share of real `.mdsmith.yml` files).
But the parity benchmark target needs both halves: Layer 0 to shed the
parse *and* a trim of the rule/per-file path (re-backing the shared
code-block-line and reference walks onto the cheap scan so they stop
re-walking, and cutting the per-diagnostic and merge overhead). Layer 0
on its own is necessary, not sufficient.

[spike-plan]: ../../../plan/2606141901_spike-block-only-parse-cost.md

### Layer 0 build: equivalence results

[Plan 2606141902][l0-plan] built the Layer 0 scanner
(`internal/lint/layer0.go`) and re-backed the block-level projections on
it. The scanner is one forward pass over `f.Lines`: it tracks fenced and
indented code blocks, processing-instruction blocks, ATX and setext
headings, block quotes, lists, HTML blocks (CommonMark types 1–7),
thematic breaks, and front matter, recording a per-line class bitfield, the
code-block and PI line sets, the block spans, and the front-matter bounds.

`CollectCodeBlockLines` and `CollectPIBlockLines` now read the cached Layer
0 scan when `f.AST` is nil (the parse-skipped path), falling back to the
AST walk when the tree is present. The equivalence harness
(`TestLayer0Equivalence_Fixtures`,
`internal/integration/layer0_equivalence_test.go`) diffs both line sets
between the AST and Layer 0 paths across every Markdown file in the
repository corpus — rule fixtures, docs, plans, and shared testdata. All
1043 corpus files produce byte-identical code-block and PI line sets, the
non-negotiable gate the migration required. The block edge cases that
needed faithful modelling matched the prediction: an info-less,
content-less fence emits no lines (goldmark exposes no source position for
it), an unclosed fence emits a phantom closing-fence line after its last
content line, indented code cannot interrupt a paragraph, and an HTML
comment makes its indented interior opaque to the code scanner.

The engine gate (`Runner.layer0SkipEligible`) skips the goldmark parse,
building the File lines-only via `lint.NewFileLines`, when the
`MDSMITH_LAYER0_SKIP` toggle is set (default off), every enabled rule
resolves to Layer 0 (the audit manifest's `A-no-skipping` rules, read
through `internal/rulelayer`), and the source carries no `<?` directive.

[l0-plan]: ../../../plan/2606141902_lazy-parse-layer0.md

### Per-rule bottleneck: line-length vs gomarklint

The parity number averages ~30 rules. To see where the floor actually
sits, the harness was re-pointed at a single-rule config (the harness
now honours `MDSMITH_SPIKE_CONFIG`): only `line-length` (MDS001), max
80 — the most-run rule, and the canonical Layer-0 rule, since its sole
AST need is the code-block line set a block scan produces. gomarklint's
matching `max-line-length` was enabled alone; it ships **off** by
default, so the parity benchmark never actually compared this rule. Same
corpus, same box.

CLI wall time (hyperfine medians). The `0 diags` rows raise the limit to
10000 so the rule still scans every line but emits nothing — isolating
the lint pipeline from output rendering; the `5455 diags` rows are the
real max-80 run:

| Run                                      | wall     | vs gomarklint |
| ---------------------------------------- | -------- | ------------- |
| gomarklint (max-line-length, 4400 diags) | ~16.7 ms | 1.0x          |
| mdsmith MDS001 block-only, 0 diags       | ~21.0 ms | ~1.26x        |
| mdsmith MDS001 block-only, 5455 diags    | ~27.8 ms | ~1.67x        |
| mdsmith MDS001 full parse, 0 diags       | ~27.5 ms | ~1.65x        |
| mdsmith MDS001 full parse, 5455 diags    | ~40.0 ms | ~2.40x        |

The gap splits into three separable bottlenecks:

1. **Inline parse — ~6.5 ms.** Block-only over full parse at equal
   output (21.0 vs 27.5 ms, 0 diags). This is what Layer 1 sheds; the
   block-only flag already removes it.
2. **Output rendering — ~6.8 ms.** The max-80 run over the 10000 run,
   block-only (27.8 vs 21.0 ms). mdsmith's default text formatter
   renders each of 5455 diagnostics with a two-line source window, a
   caret underline, and colour; gomarklint prints one terse line per
   error, so its 4400-diagnostic output is nearly free. This cost is
   orthogonal to the parse — a formatter concern that only bites on
   diagnostic-heavy files. (`-f json` is *slower* still — heavier
   serialisation — so it is no escape hatch.)
3. **Residual lint floor — ~4.3 ms (the 1.26x that survives both).**
   Block-only with *zero output* is still ~21.0 ms against gomarklint's
   ~16.7 ms. In-process (serial, output excluded) this floor is
   dominated by the **goldmark block parse — ~14 ms, which still builds
   a block *node tree*** — plus the per-file engine overhead gomarklint
   has no analog for (config resolution, gitignore, generated-section
   scan, front-matter parse, FS setup). The block-only proxy does *not*
   remove this: it suppresses the inline phase but keeps goldmark's
   block-node build.

The reading: even on the cheapest possible rule, with the inline parse
gone and *no* output, mdsmith holds a ~1.26x floor — and that floor is
almost entirely the block-node-tree build. A *true* flat Layer-0 scan
(gomarklint-style fence/heading/table tracking over `f.Lines`, no node
tree) is designed to replace exactly that. So the decisive question the
goldmark proxy cannot settle is: does a flat line classifier close the
1.26x on the pure-lint case? That is scoped as a measurement-first
prototype in [plan 2606142147][flat-l0-plan] — build the classifier,
re-back line-length on it, and re-run this head-to-head. The
diagnostic-heavy case additionally needs an output-formatter trim
(bottleneck 2), tracked there as a separate front.

[flat-l0-plan]: ../../../plan/2606142147_flat-layer0-line-classifier.md

### Flat Layer-0 result (measured)

[Plan 2606142147][flat-l0-plan] built that flat classifier and re-ran the
head-to-head. The classifier is a single forward pass over `f.Lines`
(`lint.ClassifyLines`) that tracks fenced/indented code, a blockquote/list
container stack, and marker-terminated HTML blocks — no `ast.Node`, no node
tree. Its code-block line set is **byte-identical** to the AST-derived
`CollectCodeBlockLines` across the neutral corpus (233 files), every rule
fixture (616 files), and the whole repository (1042 files) — the
equivalence gate. The engine skips the goldmark parse
(`lint.NewFileFlatPooled`) when every enabled rule is line-capable, driving
line-length from the classifier alone; its diagnostics are byte-identical
to the AST path on the corpus.

Same corpus, same 4-core box, hyperfine `--warmup 5 --runs 25 -N`, against
gomarklint v3.2.3. The `0 diags` rows raise the limit to 10000 so the rule
still scans every line but emits nothing; gomarklint's own pure-lint row
does the same. Means (± σ) and mins:

| Run                                | wall (mean) | min     | vs gml 0diag | vs gml diag |
| ---------------------------------- | ----------- | ------- | ------------ | ----------- |
| gomarklint max-line-length 0diag   | 11.2 ms     | 9.8 ms  | 1.00x        | 0.67x       |
| gomarklint max-line-length diag    | 16.6 ms     | 14.7 ms | 1.48x        | 1.00x       |
| mdsmith MDS001 flat-L0, 0 diags    | 11.6 ms     | 10.5 ms | **1.04x**    | **0.70x**   |
| mdsmith MDS001 flat-L0, diag       | 26.5 ms     | 20.5 ms | 2.37x        | 1.60x       |
| mdsmith MDS001 block-only, 0 diags | 20.0 ms     | 18.4 ms | 1.79x        | 1.20x       |
| mdsmith MDS001 full parse, 0 diags | 25.8 ms     | 24.5 ms | 2.31x        | 1.55x       |
| mdsmith MDS001 full parse, diag    | 36.5 ms     | 32.1 ms | 3.26x        | 2.20x       |

This box reproduces the per-rule study's earlier ratios against the
gomarklint-with-output baseline: block-only 0-diag lands at ~1.20–1.25x
(the study's ~1.26x) and full-parse 0-diag at ~1.55x (the study's ~1.65x),
so the new flat-L0 row is directly comparable.

**Go / no-go on the pure-lint case: GO.** The flat classifier closes the
residual the block-only proxy could not:

- Against gomarklint's *own* pure-lint time (both 0-diag, no output) the
  flat path is at statistical parity — 1.04x by mean, 1.07x by min, inside
  the run-to-run noise. Block-only sat at 1.79x and full parse at 2.31x on
  this same apples-to-apples baseline.
- Against the per-rule study's baseline (gomarklint emitting its terse
  output) the flat path is now ~1.4x **faster**, where block-only was
  ~1.26x slower. The flat classifier removes ~1.75x of pure-lint wall
  versus block-only (min 10.5 vs 18.4 ms) and ~2.3x versus full parse —
  exactly the goldmark block-node-tree build the study attributed the
  residual to.

So a true flat Layer 0 — not the block-only goldmark proxy — does reach
gomarklint-class speed on pure-lint line-length. The ~1.04–1.07x that
remains is per-file engine overhead (config resolution, gitignore,
generated-section scan, front-matter parse, FS setup) gomarklint has no
analog for, not the parse, which is gone.

The diagnostic-heavy case is now gated by **output rendering**, not the
parse: flat-L0 diag (26.5 ms) barely beats block-only diag and trails
gomarklint diag (16.6 ms) because mdsmith's text formatter renders each of
~3400 diagnostics with a source window, caret, and colour where gomarklint
prints one terse line. The parse saving is real (full-parse diag 36.5 ms →
flat-L0 diag 26.5 ms) but output dominates it. That is bottleneck 2, a
formatter concern orthogonal to the parse, scoped as a separate follow-up.

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
quantitative question — is Layer 0 + rules + overhead under
gomarklint? — is now answered by the [spike](#spike-results-measured):
**no, not on its own.** Even a free parse leaves parity ~1.4x
gomarklint, because rules + overhead alone already exceed it. Layer 0
is the right first move — it is the big win for simple configs and the
foundation for the rest — but beating gomarklint on parity needs Layer
0 *paired with* a trim of the rule and per-file path, not Layer 0
alone.

[audit]: ../../../plan/2606022126_lines-only-rule-audit.md
