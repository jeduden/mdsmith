---
id: 239
title: "cuelite phase 3 — surface C (row-expr evaluator)"
status: "✅"
model: opus
summary: >-
  Move internal/cuetemplate onto cue/cuelite (green), then flip
  catalog row-expr evaluation to an in-house tree-walking
  evaluator — string interpolation, for/if comprehensions, the
  ternary idiom, field selection, and the strings.Join and len
  builtins — checked against the CUE oracle on the real
  row-expr corpus.
depends-on: [238]
---
# cuelite phase 3 — surface C (row-expr evaluator)

## Goal

Move catalog `row-expr` evaluation onto `cue/cuelite`, then
flip it to an in-house tree-walking evaluator.

## Context

Phase 3 of [plan 218](218_wasm-size-reduction.md). Surface C
evaluates a CUE expression returning a string. It is the
richest surface but the narrowest in real use: the only live
builtin is `strings.Join`. See plan 218 for the evaluator
design.

The façade shape differs from surfaces A/B.
[cuetemplate.Compile](../internal/cuetemplate/cuetemplate.go) is
PARSE-ONLY (`parser.ParseFile`). References like `fm.id` resolve
only at `Render`, when `buildSource` injects the frontmatter
scope. `cuelite.Compile` instead EVALUATES. Feeding it a bare
row-expr would reject the unresolved references with "reference
not found". So the façade needs two new entries surface C alone
uses: a parse-without-evaluate entry (the `Compile` analogue), and
an evaluate-expression-against-a-scope entry (the `Render`
analogue that takes the frontmatter map / alias bindings).

`Template` also caches one `*cue.Context` at Compile and reuses
it across `Render` calls. That reuse does not map onto cuelite's
per-call fresh-context model. So the interim façade pays a fresh
context per catalog row until the context-free in-house evaluator
lands.

### What phase-2 `eval.go` already provides

Plan 238's flip landed a scope-threaded tree-walking evaluator,
`evalExpr(ast.Expr, scope)`. Surface C extends it, not replaces
it. It already walks these nodes:

- basic literals,
- identifiers (sibling-field references and the type keywords),
- index expressions,
- binary expressions (comparison, `&`, `|` disjunction),
- parens,
- list literals with the open tail,
- struct literals (fields, embeds, ellipsis),
- unary expressions,
- the `close` and `strings.MinRunes` calls,
- the single-clause `if` comprehension.

References resolve against `scope`. An unresolved one becomes a
thunk (`deferToThunk`) the force pass re-runs once a sibling binds.

### What surface C still has to add

The row-expr language needs four constructs `evalExpr` does NOT
yet handle:

- string INTERPOLATION (`"\(fm.id)"`) — no `*ast.Interpolation`
  case exists,
- `for` comprehension clauses — `evalComprehension` rejects
  anything but a single `if` clause,
- general SELECTOR evaluation (`fm.id`) — a bare `*ast.SelectorExpr`
  is rejected outside a builtin call, so a frontmatter field
  access does not resolve,
- a BUILTIN REGISTRY — only `close` and `strings.MinRunes` are
  wired; the live row-expr builtin `strings.Join` and `len` over a
  string/list are missing.

### Resolved in phase 2: nested-disjunction default

The phase-2 fuzzer found a disjunction-default class the in-house
engine got wrong. It was a parenthesized nested disjunction whose
value collapses to one branch (`{A: (*0|0)|10}`). Plan 238's review
round 2 fixed it in the ⟨value, default⟩ redesign. The evaluator now
keeps the sub-disjunction boundary (`flattenDisjunct`). A collapsed
sub-disjunction loses its default. The skip is deleted; surface C
inherits the corrected semantics.

## Tasks

1. [x] Add a row-expression evaluator to `cue/cuelite`: a
   dedicated tree-walker (`evalrow.go`) for the row subset, with a
   `CompileRow` (parse-only) / `RowTemplate.Render` (evaluate
   against a scope) split. It handles `*ast.Interpolation`, `+`
   concatenation, `for`/`if` comprehension clauses, general
   `*ast.SelectorExpr` and `fm["key"]` selection, and a builtin
   registry (`strings.Join`, `len`).
2. [x] Move [cuetemplate](../internal/cuetemplate/cuetemplate.go)
   onto `CompileRow`/`Render`. The suite stays green except the
   re-pins recorded below. `cuelang.org/go` is gone from
   `internal/cuetemplate` non-test files.
3. [x] Differential arm: `internal/cuelitetest/expr.go` adds an
   `ExprCase`/`ExprOutcome`/`ExprPath` arm comparing the engine
   against a direct-CUE oracle (reconstructing the former
   cuetemplate `buildSource`), plus `FuzzExpr` and a third leg of
   the `cuelite-fuzz` CI matrix.
4. [x] Gate it on the real `row-expr` in
   [markdownlint-coverage](../docs/research/markdownlint-coverage/README.md)
   and the other checked-in row-exprs, plus adversarial cases,
   checked against the CUE oracle.

## Implementation Notes

### Design decision: dedicated evaluator, not an extended `evalExpr`

The phase-2 `evalExpr` is the schema evaluator. It threads a
sibling-field scope. It defers an unresolved reference to a thunk.
The schema fuzzer pins its invariants.

Surface C has different semantics. A row reference resolves
against concrete data, or it is a hard error. There is no
deferral. The row subset also admits constructs the schema subset
must reject: interpolation, `+`, `for`, general selectors,
`strings.Join`, and `len`.

Extending `evalExpr` in place would risk those schema invariants
and tangle two scope models. So surface C got its own walker,
`evalrow.go`. It shares the value model and the literal builders
(`compileBasicLit`, `liftMapValue`), but the walk is separate and
fully covered.

### API surface added (`cue/cuelite`)

- `func CompileRow(expr string) (*RowTemplate, error)` — parse
  only (syntax check, AST cached).
- `func (*RowTemplate) Render(scope map[string]any) (string, error)`
  — evaluate against a front-matter map, yielding a concrete
  string.

The Compile/Render split mirrors `cuetemplate`. The catalog hot
path compiles a row-expr once and renders it per matched file. The
in-house engine is context-free, so a `RowTemplate` is reusable
across files and goroutines. No per-render context is allocated —
the per-call fresh-`*cue.Context` cost the old plan note feared is
gone. The schema `Compile`/`Value` API is untouched.

### Builtin registry

`rowBuiltins` is a `map[string]rowBuiltin` (`strings.Join`, `len`)
— the registry the plan asked for. Only the builtins the real row
corpus uses are wired; `len` covers string and list operands.

### Re-pinned messages

- `cuetemplate` non-concrete-string case (`"foo" | "bar"`): the
  row subset has no disjunction, so the engine reports
  `unsupported row operator` instead of CUE's `concrete string`.
  Re-pinned in `TestTemplate_Render_NonConcreteStringIsError`.
- Non-finite (`.inf`/`.nan`) and unencodable (chan) front matter:
  the in-house lifter accepts ±Inf/NaN (the schema path's
  behaviour), so the row scope adds a `checkFinite` pass and wraps
  both classes as `encoding frontmatter: field "<k>": …`,
  preserving the reject-and-name-the-field contract the catalog
  diagnostic (`TestRowExpr_NonFiniteFrontMatterNamesFile`) and
  `TestTemplate_Render_MarshalErrorReturnsError` depend on. No
  catalog test message changed; both still see `encoding
  frontmatter`.
- The non-string-result message is now the engine's
  `row expression must yield a concrete string, got …`; it still
  contains the `concrete string` substring the catalog
  `TestRowExpr_PerEntryRenderError` pins.

### Review round 1: row-evaluator semantics corrected to CUE

The round-1 review probed every row construct against the CUE oracle
(v0.16.1) and corrected the divergences the first cut carried:

- **Interpolation dialects.** `evalRowInterpolation` now ports CUE's
  `compileInterpolation`: `literal.ParseQuotes` reads the outer quote
  dialect once and `QuoteInfo.Unquote` decodes each fragment, so the
  plain, raw (`#"…"#`), and multiline (`"""…"""`) string dialects all
  render correctly — the prior fragment arithmetic fit only the
  double-quote form and silently corrupted the others. A bytes
  interpolation (`'…'`) produces CUE bytes, not a row string, and is
  rejected loudly as out-of-subset. The `Unquote` error is never
  discarded.
- **`*` repetition.** `evalRowMul` implements CUE's string×int
  repetition in either operand order (`"ab" * 3`, `3 * "ab"`); a
  negative count, a float count, int×int, and list×int reject as
  out-of-subset. The CI `FuzzExpr` crasher (`"" * 0`, empty scope) is a
  committed seed.
- **Equality.** `rowEqual` matches CUE's two rules: a top-level scalar
  pair is numeric-aware (`2 == 2.0` true) while a list or struct
  compares structurally with kind-strict element equality
  (`concreteValueEqual`): `{k:1} == {k:1}` true, `[2] == [2.0]` false.
- **`len`.** `len(string)` is a BYTE count, not a rune count, matching
  CUE (`len("café")` is 5). `len(struct)` stays a loud out-of-subset
  rejection.
- **Quoted selectors.** `fm."my-key"` and `fm."?"` resolve the quoted
  string member instead of the schema path's `"?"` fallback.
- **Big-int / float arithmetic policy.** int+int `+` is overflow-checked
  (an int64 overflow is out-of-subset, consistent with the big-literal
  policy); float `+` (any float operand) is rejected loudly — the
  float64 engine would render `0.1 + 0.2` as noise where CUE keeps a
  decimal, and the real corpus never adds floats. Display-interpolation
  of a float VALUE is unaffected.

### Review round 2: oracle alignment and four more CUE divergences

Round 2 re-probed every item against the oracle (v0.16.1) and fixed five
issues round 1 left:

- **Oracle missed `mdsmith_row_out`.** It aliased and exposed the
  in-house parse-wrapper field, so a scope key of that name diverged.
  `RowScaffoldFieldNames` now single-sources the scaffolding names.
- **Quoted hidden selectors.** `fm."_key"` selects the `_`-field per CUE;
  only a bare-ident `fm._key` is hidden. `evalRowSelector` applies the
  `_`-prefix rejection only to the bare form.
- **Builtin shadowing.** A scope key or for-variable named `len` shadows
  the `len` builtin lexically; `strings` stays reserved. `evalRowCall`
  resolves a bare-ident target against scope first.
- **Unary `+`.** CUE accepts `+(1+2)` as a numeric identity
  (`identityNumeric`). The unsupported-construct hatch doc now enumerates
  its real members, adding the d45b673 repetition-bound rejection, one
  `FuzzExpr` seed per member.
- **Unary `-` of int64 min** (sign-off round). The row `-` arm wrapped
  silently where CUE yields `9223372036854775808`. It now rejects as
  out-of-subset, like `checkedAddInt64`; a test and a seed pin it.

### Binding contract (newRowScope)

The scope binding matches the CUE oracle on every reference form the
corpus and `FuzzExpr` exercise. Round 1 stated it "matched the oracle
exactly"; round 2 found that overstated and fixed the last divergence
(below). The contract is:

- A key binds as a BARE identifier only when it is a CUE-safe identifier
  (`^[A-Za-z][A-Za-z0-9_]*$`) that is not reserved. Reserved are `fm`,
  the `strings` builtin namespace, the CUE keywords, and the two
  scaffolding field names. A `_`-prefixed (hidden) key, a non-identifier
  key, a keyword, and `strings`/`fm` get NO bare alias.
- The whole map binds under `fm`, minus a literal `fm` key and the
  scaffolding keys (the `fm` binding always wins). A `_`-prefixed member
  is reachable via a string INDEX (`fm["_key"]`) but not a bare SELECTOR
  (`fm._key`), as CUE hides `_`-prefixed fields from selection.

The differential ORACLE was fixed to implement the SAME contract. The
leaky `_strings_used` sink is gone. A two-pass compile adds the
`strings` import only when the expression uses it, so no extra name
exists to reference. The oracle reserves and drops the same `fm` and
scaffolding keys the in-house arm does, via the shared
`RowScaffoldFieldNames` source (round 2; see above).

### Hatch redesign (divergence-scoped)

The former float hatch was scope-scoped. It masked any string diff when
the scope held a fractional number. Two signature-matched classes in
`HatchedDivergence` replace it:

- **float-display**: both arms accept and the only differences are
  numeric substrings whose parsed values are equal-ish — the float64
  vs decimal rendering divergence, and nothing else.
- **unsupported-construct**: the in-house arm rejects with the
  "unsupported" wording while CUE accepts — for…if clauses,
  `for i, x in`, multi-clause `let`, len(struct), struct literals,
  int64 overflow (`+`, unary `-` of int64 min), bytes interpolation,
  float arithmetic, over-bound repetition; each member is seeded. It
  never masks any other divergence shape.

### Differential finding

The differential arm caught a real int/float divergence. A JSON
scope decoded with the default `json.Unmarshal` turns `42` into a
`float64`. The engine then interpolates it as `42.0`, while CUE
prints `42`. The real front-matter path keeps integers as
integers. So the harness decodes scope JSON with `UseNumber`, and
the in-house lifter keeps the int. That removes the artifact.

One genuine divergence remains. A fractional float interpolates
literal-preserving in CUE (`2.0`, `1.50`) but shortest-round-trip
in the engine. `FuzzExpr` hatches it via the float-display class.
The real corpus never interpolates a float.

### Deviations from the rewritten plan

- The plan's task 1 described extending `evalExpr` with the four
  constructs; the implementation adds a separate `evalrow.go`
  walker instead (rationale above). The four constructs are all
  present, just in the row walker.
- Three unreachable defensive branches were NOT added (per the
  repo's "drive it red/green" rule): a non-field row declaration,
  a non-struct comprehension value, and `true`/`false`/`null` as
  identifiers (the parser emits them as basic literals). The CUE
  grammar guarantees none can fire.

## Acceptance Criteria

- [x] `internal/cuetemplate` imports `cue/cuelite`, not
      `cuelang.org/go` (non-test files).
- [x] The in-house evaluator matches CUE on every checked-in
      `row-expr` (differential corpus + `FuzzExpr` 30 s smoke).
- [x] `cue/cuelite` evaluator code keeps 100 % statement
      coverage; `internal/cuelitetest` is 100 % too.
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues on the touched
      packages.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
