---
summary: >-
  Comparison of cue/cuelite — mdsmith's in-house, stdlib-only
  CUE-subset engine — against the cuelang.org/go dependency it
  replaced, with the measured deltas and the process learnings
  from the five-phase replacement (plans 218, 236–240).
---
# cuelite vs CUE

This research note compares
[`cue/cuelite`](../../../cue/cuelite/doc.go) — the in-house,
pure-Go, standard-library-only engine that mdsmith ships since
plan 240 — against `cuelang.org/go v0.16.1`, the upstream CUE
implementation it replaced. The first half is the comparison:
what each engine is, what the swap measurably changed, and what
it cost. The second half records the process learnings from the
replacement project ([plan 218](../../../plan/218_wasm-size-reduction.md),
phases [236](../../../plan/236_cuelite-package-harness.md)
through [240](../../../plan/240_cuelite-drop-cue.md)),
so the next dependency-replacement effort starts from evidence
rather than memory.

## Why mdsmith replaced CUE

mdsmith never used CUE the language platform. It used a small,
fixed API over four surfaces:

- **A. Schema constraints (MDS020)** — compile a per-key
  constraint struct from `frontmatter:` values, unify with the
  document's front matter, check `Concrete(true)`.
- **B. `query` / catalog `where:`** — the same unify-and-check,
  plus a leaf-path existence requirement. A strict subset of A.
- **C. Catalog `row-expr` templates** — evaluate a CUE
  expression returning a string: interpolation `\(x)`,
  comprehensions, the `[if c {a}, if !c {b}][0]` ternary idiom,
  field selection, `strings.Join`.
- **D. Placeholder paths** — `cue.ParsePath` for `{a.b.c}` and
  `{"my-key".sub}` placeholders.

Seven non-test files imported the dependency. The price was the
whole platform: about 95 packages plus `cockroachdb/apd`
(arbitrary-precision decimals) and protobuf. That weight set the
WASM artifact size (~37.9 MB raw), blocked the tinygo toolchain
outright, and put a JSON round-trip plus context plumbing on the
per-file validation hot path. None of CUE's lattice machinery
beyond the subset was reachable from mdsmith input: front-matter
values fit int64/float64, and `=~` is RE2 — Go's `regexp`.

## The comparison

### Scope and semantics

cuelite is not a CUE implementation. It is the exact subset the
four surfaces use, with semantics pinned to `cuelang.org/go
v0.16.1` by a differential harness that ran both engines over
the full corpus before the dependency was deleted. Three design
positions follow from that:

- **Identical syntax on the subset.** Every schema, `proto.md`,
  plan front matter, query, and row template that worked under
  CUE works unchanged. `mdsmith check .` needed no migration.
- **Loud rejection off the subset.** A construct cuelite does
  not implement fails with an out-of-subset error; it never
  silently approximates. Float `+` rejects (a float64 engine
  would render `0.1 + 0.2` as noise where CUE keeps a decimal),
  bytes interpolation rejects, `len(struct)` rejects, int64
  overflow rejects. The hatch's members are enumerated, tested,
  and fuzz-seeded.
- **Frozen target.** cuelite tracks v0.16.1 behaviour. If
  upstream CUE changes semantics, cuelite does not follow; the
  pinned corpus is the contract.

### Measured deltas

| Dimension                | CUE (`cuelang.org/go v0.16.1`)                        | cuelite                                                                                                  |
| ------------------------ | ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Dependency graph         | ~95 packages + `cockroachdb/apd` + protobuf           | standard library only                                                                                    |
| Engine size              | full language platform                                | ~8.9 k source lines (+ ~7.9 k test lines)                                                                |
| WASM artifact (stripped) | ~37.9 MB raw                                          | ~11.2 MB raw / ~2.8 MB gzipped                                                                           |
| Validate hot path        | ~33 µs/op, ~213 allocs/op, JSON round-trip per check  | 7.9 µs/op, 85 allocs/op, `CompileMap` direct on `map[string]any`                                         |
| Cold compile+validate    | ~63 µs/op                                             | 22.1 µs/op, 205 allocs/op                                                                                |
| Number model             | arbitrary-precision decimal (apd)                     | int64 / float64, overflow-checked                                                                        |
| Value lifetime           | context-bound, not concurrency-safe across goroutines | immutable value type, shareable, compile-once                                                            |
| tinygo                   | does not compile                                      | engine compiles; full binary blocked on `os.*` gaps ([plan 247](../../../plan/247_tinygo-wasm-build.md)) |

Numbers come from the recorded phase results: the hot path holds
0.20–0.30× CUE's time and 0.40× its allocations
([plan 238](../../../plan/238_cuelite-surfaces-ab.md), task 4),
and the size and allocation ceilings are regression-guarded in
CI (`cmd/mdsmith-wasm/size_test.go`, `cue/cuelite/bench_test.go`).
On the real workspace, `mdsmith check .` ran ~18 % faster after
the flip, mostly from dropping the per-check `json.Marshal` and
the per-value `*cue.Context`.

### What the swap cost

The trade is not free, and the costs are worth naming as
plainly as the wins:

- **A parser is now owned code.** The phase-4 syntax frontend
  (scanner, parser, string/number decoding across the plain,
  raw, and multiline dialects) is the kind of code that hides
  wrong-value bugs — the review rounds below found several.
- **Semantics are frozen.** New upstream CUE features never
  arrive. For mdsmith that is the point; for a consumer who
  wants the language, it is a wall.
- **The subset boundary needs policing.** Every out-of-subset
  construct must reject loudly, which means the boundary itself
  is test surface (one fuzz seed per hatch member).
- **Five plans, five PRs, two review rounds per phase.** The
  replacement ran as plans 236–240 with an adversarial review
  round after each implementation cut. That process cost is the
  reason the cut-over produced byte-identical MDS020
  diagnostics.

## Learnings

### Adopt, then flip — never both at once

The single most load-bearing decision was splitting the two
risks. `cue/cuelite` landed first as a thin façade **over CUE**;
every call site moved onto it while behaviour stayed
green-by-construction. Only then was the engine flipped behind
the stable API, surface by surface (D, then A+B, then C, then
the parser). When a diff contains either a call-site move or an
engine change but not both, every regression has one suspect.

One honest caveat from the record: phase 1 did not follow the
cadence. Plan 237's deviations section records that tasks 1–3
were collapsed — `fieldinterp` moved onto `cuelite.ParsePath`
and that `ParsePath` was written in-house in a single pass, with
no CUE-delegating intermediate ever committed. The substitute
safety was the differential fuzzer, and the six review rounds it
took to converge (see the appendix) are a fair price tag for the
shortcut. The later, larger phases followed the cadence.

The façade was shaped for the flip from day one: `Value` is a
value type whose bottom (⊥) absorbs cleanly, so the CUE-backed
phase-0 struct and the flipped in-house struct share one API
with no nil-receiver hazard and no signature change.

### A differential oracle is the correctness instrument

Hand-written expectations encode the author's misunderstanding.
The harness in `internal/cuelitetest` instead asked CUE itself:
both engines ran the same corpus — every `frontmatter:`
constraint in the repo, the file-kinds conflict table, the query
examples, the row-expr suite, and fuzz-generated schema×data
pairs — and any accept/reject or error-path disagreement failed
the build. Two non-obvious corollaries:

- **The oracle needs auditing too.** A round-2 review found the
  oracle itself leaking a scaffolding field
  (`mdsmith_row_out`) into the comparison scope, masking a
  divergence ([plan 239](../../../plan/239_cuelite-surface-c.md),
  review round 2). Differential testing only proves agreement
  with the oracle as written.
- **Compare at the right granularity.** Each harness outcome
  carries a `Stage` discriminator (compile-schema, compile-data,
  validate, accepted, error), so a schema the in-house engine
  cannot parse can never look like agreement with an oracle that
  merely rejected the data
  ([plan 236](../../../plan/236_cuelite-package-harness.md)).
- **Build the scaffolding for deletion.** A draft put the
  harness in a public `cue/cuelite/difftest` package; review
  moved it to `internal/cuelitetest` because a public harness
  would let external users depend on a package phase 4 deletes
  and would pin `cuelang.org` into `go.mod` after the flip. The
  oracle was deletable precisely because nothing outside the
  module could see it.
- **Retire the oracle into pinned corpora.** When CUE left
  `go.mod`, the harness's purpose ended — but its corpus did
  not. Every differential case became an engine-only pinned
  test (`corpus_test.go`, `rowcorpus_test.go`), with the header
  stating the rows were validated against the oracle while it
  existed. The proof outlived the instrument.

### Adversarial review rounds catch what tests and fuzzing miss

Each phase got at least one review round whose brief was to
probe the implementation against the oracle construct by
construct, assuming the first cut was wrong. The yield was
consistently bugs that the (already 100 %-covered, fuzzed) test
suite had encoded rather than caught — wrong-value acceptances,
not crashes:

- **Parser (phase 4, commit `c78b1b7`):** raw strings
  (`#"…"#`) failed to round-trip; `010` parsed as octal 8
  instead of rejecting; `1.5e-2` silently lost its exponent to
  a short-circuited `fraction || exponent`; run-together
  declarations (`a: 1 b: 2`) were accepted; under-indented
  multiline interiors passed as silent `TrimPrefix` no-ops.
- **Row evaluator (phase 3):** the first interpolation cut fit
  only double-quoted strings and silently corrupted the raw and
  multiline dialects; `len(string)` counted runes where CUE
  counts bytes; `2 == 2.0` and `[2] == [2.0]` follow different
  equality rules; a scope key named `len` must shadow the
  builtin; unary `-` of int64 min wrapped silently.
- **Engine (phase 2):** chained `Unify` could drop constraints;
  disjunction defaults needed CUE's per-disjunct modes rather
  than flatten-and-mark, because `(*0|0)|10` loses its default
  when flattened by value.

The pattern: coverage proves the code runs, fuzzing proves it
does not crash, and only oracle-anchored adversarial probing
proves the values are right. For an engine whose failure mode is
a wrong accept, the third leg is the one that matters. The
complete per-phase fix inventory is in the appendix below.

Two refinements from the phase-1 record
([plan 237](../../../plan/237_cuelite-surface-d.md)):

- **A fuzzer explores around its seeds, not the whole space.**
  The differential fuzzer's mutations were seeded from the known
  divergence classes, so it probed the byte-space around them
  but did not compose new ones. Review round 3 found a class it
  had never reached (raw-string × surrogate-escape pairing,
  hiding a reachable out-of-bounds panic). Each review round
  then re-seeded the fuzzer with the new class, and a timed deep
  fuzz run after every fix batch was the convergence check.
- **The fixes need review too.** Round 4's CR handling was
  itself wrong: it stripped `\r` before lexing where CUE strips
  it only after scanning, so three inputs CUE rejects were
  accepted. Round 5 corrected it (validate the raw bytes, strip
  for value assembly) and round 6's longer fuzz run still found
  one more member of the same family. A review round is a cut
  like any other, not a stamp.

### Convert relative gates to absolute gates before deleting the baseline

The performance gate started as a factor gate: cuelite must stay
under 1.0× CUE's time and allocations. That gate divides by the
oracle, so it dies with the oracle. Before deletion it was
converted to an absolute allocs/op ceiling
(`maxValidateAllocs = 60` in `bench_test.go`) set just above the
measured value. The general rule: any guard expressed relative
to a thing being removed must be re-expressed in absolute terms
first, or the protection silently leaves with the dependency.

While it lived, the factor gate also taught a measurement
lesson: a ratio is only meaningful on a quiet runner. Under the
parallel test job's CPU contention the cuelite arm degraded more
than the oracle arm and the factor inflated to an observed
3.46×, so the gate armed only via `CUELITE_FACTOR_GATE=1` in a
dedicated serial CI job and skipped everywhere else
([plan 236](../../../plan/236_cuelite-package-harness.md)).

### Pinned contracts steer the work — change them only with sign-off

Two episodes show the same rule from opposite sides. When the
phase-2 flip replaced the façade's interim cross-context
behaviour with single-context CUE semantics, four pinned test
classes had to be rewritten — and that happened as an explicit,
recorded authorization (the "test-contract flip" in
[plan 238](../../../plan/238_cuelite-surfaces-ab.md), task 3)
rather than a quiet edit. Conversely, phase 1 skipped a cleanup
(threading `ParsePath`'s precise error through `fieldinterp`)
because seven call sites and an acceptance test pin the
user-visible error message; changing a pinned message contract
was outside the round's mandate, so the item was deferred, not
smuggled in. The pins did their job in both directions: they
made every behaviour change loud.

### Fuzz crashers are permanent corpus, not fixed incidents

Every fuzzer finding (`"" * 0` with an empty scope, the
`{B:0>0>A,A:0}` reference cycles, the deep array-element
duplicate key) became three artifacts: a committed fuzz seed, a
pinned corpus row, and a dedicated test. CI runs three smoke
fuzzers (`FuzzRowSmoke`, `FuzzSchemaSmoke`, `FuzzPathSmoke`) on
every PR and uploads any crasher as a build artifact. A crasher
that is merely fixed can regress; a crasher that becomes corpus
cannot regress silently.

### 100 % coverage as a design forcing function

The package targets 100 % statement coverage, paired with the
house rule that a defensive branch may exist only if a test can
drive it red/green. The combination is stronger than either
alone: every unreachable branch had to be either made reachable
(and tested) or deleted. The engine's error paths are all real.
One operational footnote: the project's CI measures per-package
coverage without `-coverpkg`, so cross-package incidental
coverage does not count — the syntax package needed its own
tests for lines the parent package's corpus already exercised.

### Honest acceptance criteria survive contact with reality

The tinygo criterion failed: with CUE gone and the
`sync.Map.CompareAndDelete` lever replaced, `tinygo build`
still dies on `os.Chmod` / `os.SameFile` / `os.Symlink` calls
reached transitively from `pkg/mdsmith`. The criterion stayed
`🔲` with the verified error inventory written into the plan,
the size test records the failure and skips rather than faking a
pass, the CI job runs informationally, and the remaining work
became [plan 247](../../../plan/247_tinygo-wasm-build.md) with
the inventory as its task list. A criterion quietly weakened to
pass tells the next reader nothing; a criterion that fails
loudly with evidence is a plan.

## When a subset engine is the right call

Generalizing from this case, replacing a dependency with an
in-house subset engine paid because all of these held:

- the **used surface was small and enumerable** (seven import
  sites, four surfaces, a fixed grammar);
- the **dependency's cost was structural**, not incidental —
  artifact size, a blocked toolchain, and hot-path overhead
  that no amount of tuning around the API could remove;
- an **executable oracle existed** to pin semantics against,
  for as long as the replacement was being proven;
- the **domain bounded the hard parts** — int64/float64 cover
  front matter, so the bignum engine that justifies much of
  CUE's weight was never needed;
- the team could afford **loud rejection** at the subset
  boundary instead of compatibility.

Absent any one of these — a sprawling used surface, no oracle,
or a domain that needs the dependency's hardest 20 % — the same
move would produce a permanently diverging fork rather than a
small engine with a frozen contract.

## Appendix — the complete divergence-fix record, by phase

Every oracle-confirmed divergence between the first in-house cut
and CUE v0.16.1, with where the full account lives. Each fix was
pinned three ways — corpus row, unit table, fuzz seed — before
its review round closed.

### Phase 1 — `ParsePath` ([plan 237](../../../plan/237_cuelite-surface-d.md), six review rounds)

- **Round 1.** The committed parser used an ASCII-only
  identifier scanner and a corpus chosen to avoid known
  divergences; fuzzing against the oracle exposed the
  systematic gap and the identifier rules were corrected.
- **Round 3** (raw-string × surrogate-escape pairing, a class
  the fuzzer had never reached): a reachable out-of-bounds
  panic on a high surrogate half before the closing delimiter;
  a valid `\#u`+`\#u` surrogate pair wrongly rejected; an
  invalid `\#u`+plain-`\u` pair wrongly accepted.
- **Round 4.** The single-line raw-string close scan used a
  blind index, so an escaped quote followed by a hash run
  (`#"\#"#"#`) read as a false terminator — CUE accepts these;
  the scan became escape-aware. Multiline string labels
  (`"""…"""`, plain and raw) were rejected wholesale where CUE
  accepts them as head and bracket operands; they were
  implemented porting `cue/literal`'s indentation, final-newline,
  and escape semantics.
- **Round 5** (correcting round 4's CR model): CUE strips `\r`
  from the scanned literal only AFTER scanning, while the
  opener-newline check and escape scanning run on raw bytes. So
  a CR run at the opener, a raw CR between backslash and escape
  selector, and a raw CR among `\u` hex digits are CUE scan
  errors the in-house parser had accepted. The same round
  ported the `\#`+newline escaped-newline asymmetry (CUE's
  scanner and `literal.Unquote` disagree; the value path is
  reachable via `\`+CR+`#` fusion) including line-continuation,
  final-newline, and bad-indent handling. Branch coverage rose
  from 342/344 to 394/396.
- **Round 6.** A 420 s fuzz run found one more CR-family member:
  a multiline close fused from `"""`+CR+`#` by CR-stripping,
  which CUE's raw-byte scanner does not treat as a close. The
  close now matches on raw bytes.

### Phase 2 — schema engine ([plan 238](../../../plan/238_cuelite-surfaces-ab.md))

- **Chained-unify compositions.** The façade's interim
  cross-context behaviour diverged from single-context CUE; the
  flip restored single-context semantics as the truth, with an
  oracle test that rebuilds each chained composition inside one
  `cue.Context` and asserts matching accept/reject.
- **Disjunction-default semantics** (the ⟨value, default⟩
  redesign): flatten-and-mark lost a collapsed sub-disjunction's
  default (`(*0|0)|10` wrongly rejected); replaced with CUE's
  per-disjunct modes. `unifyDisjunction` now prunes a
  nested-bottom branch and reconciles the meet default from
  operand defaults; `concreteValueEqual` covers `*[]`; thunk
  fixpointing and a thunk-`&` deferral complete the family.
- **Strict-JSON duplicate keys.** CUE's JSON lift unifies
  same-named object keys into a phantom merged object no
  last-wins JSON reader would build; `CompileJSON` rejects any
  duplicate key before the lift instead.
- The 600 s differential fuzz found one further divergence,
  fixed and seeded.

### Phase 3 — row evaluator ([plan 239](../../../plan/239_cuelite-surface-c.md), two review rounds)

- **Round 1:** interpolation handled only the double-quote
  dialect and silently corrupted raw (`#"…"#`) and multiline
  (`"""…"""`) strings — replaced with CUE's quote-dialect
  decoding; bytes interpolation rejects loudly. `"ab" * 3`
  repetition works in either operand order; negative, float,
  int×int, and list×int counts reject. Equality follows CUE's
  two rules: top-level scalars numeric-aware (`2 == 2.0` true),
  lists/structs kind-strict (`[2] == [2.0]` false).
  `len(string)` counts bytes, not runes. `fm."my-key"` resolves
  the quoted member instead of a fallback. Int `+` is
  overflow-checked; float `+` rejects loudly (float64 would
  render `0.1 + 0.2` as noise where CUE keeps a decimal).
- **Round 2:** the ORACLE leaked its `mdsmith_row_out`
  scaffolding field into scope, masking a divergence — the
  scaffolding names became single-sourced. `fm."_key"` selects
  the `_`-field per CUE; only bare-ident `fm._key` is hidden. A
  scope key named `len` shadows the builtin lexically. Unary
  `+(1+2)` is a numeric identity per CUE. Unary `-` of int64
  min wrapped silently where CUE yields the exact value; it now
  rejects as out-of-subset.

### Phase 4 — parser ([plan 240](../../../plan/240_cuelite-drop-cue.md), review commit `c78b1b7`)

- Raw strings (`#"…"#`) failed to round-trip: the scanner
  returned the token starting after the consumed `#` run, so
  the decoder saw an unterminated literal.
- `010` parsed as octal 8 (a wrong-VALUE acceptance);
  malformed underscores (`1_`, `1__2`, `0x12_`) and the
  `0O`/`0B` prefixes now reject like CUE, while a leading-zero
  float stays legal. The same fix exposed a latent bug:
  `1.5e-2` lost its exponent to a short-circuited
  `fraction || exponent`.
- Run-together declarations (`a: 1 b: 2` on one line) were
  accepted; declarations now require a comma or newline.
- A stray `?` after a label errored nowhere and was silently
  dropped; it now reports.
- An under-indented multiline interior line and a closing `"""`
  not on its own line passed as silent trim no-ops; both are
  now errors.
- The `FuzzValidate` crashers (`{B:0>0>A,A:0}` reference cycles
  and a deep array-element duplicate key) were re-pinned as
  corpus rows, seeds, and a dedicated test when the oracle
  harness was retired.

## See also

- [Plan 218 — the umbrella plan](../../../plan/218_wasm-size-reduction.md)
- [Plans 236–240 — the five phases](../../../plan/236_cuelite-package-harness.md)
- [Plan 247 — the tinygo follow-up](../../../plan/247_tinygo-wasm-build.md)
- [Engine API and WASM budgets](../../background/concepts/engine-api.md)
- [Schema-unification spike](../schema-unification/spike.md) —
  the earlier research that kept CUE syntax and rejected a YAML
  DSL; cuelite implements that decision without the dependency.
