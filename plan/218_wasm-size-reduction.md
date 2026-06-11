---
id: 218
title: In-house CUE-subset engine for WASM size and tinygo
status: "🔳"
model: opus
summary: >-
  Replace the cuelang.org/go dependency with a small, pure-Go,
  stdlib-only engine implementing the exact CUE subset mdsmith
  uses — schema constraints (MDS020), `query`/`where:` filters,
  `{field}` placeholder paths, and catalog `row-expr` templates —
  with identical syntax and semantics. The package lands first as
  a CUE-backed façade, is adopted across mdsmith, then flipped to
  the in-house engine surface by surface, proven by differential
  tests against CUE. Drops ~95 packages plus bignum and protobuf,
  unblocks tinygo, brings the WASM artifact under the plan-215
  budgets, and removes per-check JSON round-trips and per-context
  value plumbing from the hot path. Ships as a public,
  benchmarked Go package. Split into phase plans 236–240.
depends-on: [215]
---
# In-house CUE-subset engine for WASM size and tinygo

## Goal

Drop `cuelang.org/go`. Replace it with a small in-house
engine: pure Go, standard library only. It covers the exact
CUE subset mdsmith uses, with the same syntax and semantics.
Four payoffs follow:

- the dependency leaves `go.mod`;
- the WASM artifact fits the plan-215 budgets;
- tinygo can compile the engine;
- the schema hot path loses a JSON round-trip per check.

## Background

mdsmith pulls in all of CUE for a tiny, fixed API. The cost
is `cuelang.org/go v0.16.1`: about 95 packages, plus
`cockroachdb/apd` (bignum) and protobuf. Yet only **7
non-test files** import it, across four sub-packages (`cue`,
`cue/cuecontext`, `cue/parser`, `cue/errors`). Nothing uses
`cue/ast`, `cue/format`, `cue/load`, the module loader, or
user-facing `import`/`package`/`#def` syntax.

The API surface in use is small: construct a value
(`CompileString`, `CompileBytes`), unify and validate it
(`Unify`, `Validate(Concrete(true))`), and read it
(`LookupPath`, `String`, `Decode`, `Exists`, `Fields`). Paths
use `ParsePath`. Diagnostics use `errors.Error.Path`. The
façade below mirrors exactly this set.

CUE is used on exactly four surfaces.

**A. Schema constraints (MDS020).**
[internal/schema](../internal/schema/validate.go) and
[requiredstructure](../internal/rules/requiredstructure/rule.go)
compile a per-key constraint struct from the `frontmatter:`
values, unify it with the document's front matter, and check
`Concrete(true)`. The features used: the type atoms (`string
int float number bool bytes null _`), `=~` regex, `&`, `|`,
the bounds `>= <= > < !=`, the `*` default, trailing `?`
optional keys, struct literals with `close()`, lists
(`[...T]`, `[_, ...T]`), `len()`, and `strings.MinRunes`.

**B. `query` / catalog `where:`.**
[internal/query](../internal/query/query.go) wraps the
expression in `{...}`. It requires every referenced leaf path
to **exist**, because CUE structs are open. Then it unifies
and checks concreteness. This is a strict subset of A.

**C. Catalog `row-expr` templates.**
[internal/cuetemplate](../internal/cuetemplate/cuetemplate.go)
evaluates a CUE *expression returning a string*: string
interpolation `\(x)`, list comprehensions, the `[if c {a}, if
!c {b}][0]` ternary idiom, field selection, and
`strings.Join`. This is the richest surface, but the
narrowest in real use.

**D. Placeholder paths.**
[internal/fieldinterp](../internal/fieldinterp/fieldinterp.go)
uses only `cue.ParsePath`, to parse `{a.b.c}` and
`{"my-key".sub}`. Resolution is already hand-rolled.

The seven field-type shortcuts in
[cue/types/types.cue](../cue/types/types.cue) are one-liners:
six `=~` regexes and one `string & !=""`. They are read as
text, not imported as CUE.

Why this matters now: the
[engine-api page](../docs/background/concepts/engine-api.md)
names CUE as the dominant WASM cost (~40 MB raw / 8.6 MB
gzipped). It also blocks tinygo. mdsmith needs only
int64/float64 bounds and RE2 regex. Go's `regexp` backs `=~`.
So none of CUE's lattice machinery is required.

### Strategy: one engine, adopted then flipped

One engine everywhere beats a build-tagged, wasm-only subset.
It keeps one code path, drops the dependency on every target,
and speeds the native CLI and LSP. Getting there safely splits
two risks: moving call sites, and engine correctness.

So `cue/cuelite` lands first as a **thin wrapper over CUE**.
Every call site moves onto it unchanged — that stays green.
Only then is it **flipped** to the in-house engine behind the
stable API. The CUE-backed path stays as the differential
oracle. The moves are red/green TDD, surface by surface.

This is **not** the schema-unification
"[Direction B](../docs/research/schema-unification/spike.md)"
that swaps CUE syntax for a YAML DSL: same spec, CUE syntax
stays valid everywhere.

## Design

### New package

A public leaf package, proposed `cue/cuelite` beside
[cue/types](../cue/types/types.cue), exported and versioned
like [pkg/markdown](../docs/development/markdown-library.md). Once flipped
it imports only the standard library: `regexp`, `strconv`, `sort`,
`unicode/utf8`, and optionally `encoding/json`. It sits at the
bottom of the layering map. Its consumers are the `schema`,
`requiredstructure`, `query`, `fieldinterp`, and `cuetemplate`
packages; it imports none of them. The façade mirrors the CUE
calls above:

```go
func Compile(src string) (Value, error)      // CompileString
func CompileJSON(data []byte) (Value, error) // strict JSON; stricter than CompileBytes
func ParsePath(expr string) (Path, error)    // {a.b.c} → Path
func MakePath(segments ...string) Path        // construct from segments
func (v Value) Unify(o Value) Value
func (v Value) Validate() error              // Concrete(true)
func (v Value) LookupPath(p Path) (Value, bool)
func (v Value) String() (string, error)
func (p Path) Segments() []string            // unquoted per-selector strings
// plus Decode, Exists, Fields; errors carry a Path. MakePath serves
// query.collectPaths; Segments serves fieldinterp.ParseCUEPath.
```

`Value` is a **value type**, not a pointer. Methods take and
return `Value` by copy. A zero/bottom `Value` composes without a
nil receiver to panic on, and the future hot path pays no heap
allocation per call (the ≤ 10 allocs/op budget). A bottom (⊥)
absorbs cleanly whether it is a phase-0 error-carrying struct or
a flipped in-house `Value`. So the API shape is identical before
and after the flip.

One simplification falls out. A CUE value is tied to a
`*cue.Context` and cannot cross contexts. That forces the
context-pairing plumbing in
[compile_cache.go](../internal/schema/compile_cache.go).
The phase-0 façade pays an honest interim cost for this. Each
compiled `Value` owns a fresh `*cue.Context`, since CUE v0.16.1
documents that values from one context are neither
concurrency-safe nor memory-bounded. `Unify` rebuilds whichever
side retains source into the OTHER side's mutated context, so a
shared schema needs synchronization and a long-lived `Value`
grows until the flip.

The flipped in-house `Value` is a context-free immutable struct.
A compiled schema is then shareable across goroutines, and the
per-`Value` context disappears with no API change.

The differential oracle harness lives under
`internal/cuelitetest`. It is module-internal, so the
`cuelang.org/go` import it carries never reaches the public
surface and is deleted with the package in phase 4.

### Value model

A `Value` is one of a fixed set of shapes:

- bottom (⊥), carrying a reason and a path;
- top (`_`), and null;
- a concrete scalar: string, int64, float64, bool, or bytes;
- a typed atom (the kind, no value yet);
- a bounded scalar (kind plus the set `>= <= > < != =~`);
- a disjunction (branches, plus an optional default for `*`);
- a struct (ordered fields, each optional?, open or closed);
- a list (`[...T]`, or `[_, ...T]` with a required prefix length).

Numbers are int64/float64; front-matter and JSON values fit, so no bignum.

### Unification and concreteness

`Unify` is the lattice meet over that model. The rules are
small:

- ⊥ absorbs; ⊤ is identity. Concrete & concrete must be equal.
- concrete & bound/type must satisfy; bound & bound intersect on a shared kind.
- a disjunction distributes: drop ⊥ branches, collapse a
  singleton, preserve a default.
- structs unify field-wise (an extra field vs a closed struct
  is ⊥); lists unify by element and length.

`Validate` reports one `*PathError` per non-concrete or ⊥
leaf. Each error is tagged with its field path, matching CUE's
`Validate(Concrete(true))`. A single failing leaf returns a bare
`*PathError`; several return an `errors.Join` of them. The
package-level `Errors` accessor flattens that bare-or-joined
error into the full slice of per-field failures. A consumer thus
enumerates every rejecting leaf without type-switching on the
join shape.

### Hot-path performance

- **Skip the JSON round-trip.** Today every check does
  `json.Marshal(fm)` → `CompileBytes` → `Unify`. The engine
  validates a `map[string]any` directly instead, cutting the
  marshal and most allocations. `CompileJSON` is off the hot path.
- **Compile once, share freely.** Regexes compile at
  schema-compile time and cache on the node. The immutable
  `Value` is reused across files and goroutines through the
  [RunCache](../internal/lint/runcache.go).
- **Budget.** Validation stays within an absolute allocs/op
  guard (`cue/cuelite/bench_test.go`).

### Expression evaluator (surface C)

Surface C needs a real evaluator, so it gets its own phase. It
is a small tree-walker over the same parsed AST. It supports
string interpolation, `for`/`if` comprehensions, indexing,
field selection, and the operators the ternary idiom needs. It
also carries a tiny builtin registry: `strings.Join` and
`len`, and reuses cuetemplate's scope binding.

### Testing and coverage

`cue/cuelite` targets **100 % statement and branch coverage**.
The [coverage gate](../docs/development/coverage.md) holds the
patch at the project baseline. The four-layer
[pyramid](../docs/development/architecture/tests.md) applies:

- **unit** — a dedicated test per function, with every `Unify`
  and `Validate` rule and each ⊥/error path driven red/green, so
  no defensive branch is left uncovered;
- **contract** — the façade surface and the `errors`/`Path`
  shape MDS020 reads;
- **integration** — the differential harness ran the in-house
  path against an **independent direct-CUE oracle** over every
  `frontmatter:` constraint, the
  [file-kinds conflict table](../docs/guides/file-kinds.md), the
  query/`where:` examples, and a fuzzer, before CUE was removed;
- **e2e** — `mdsmith check .`, unchanged.

CUE was removed in phase 4 (plan 240), once the diff was clean.

### WASM / tinygo

With CUE gone, the engine is pure stdlib. One more change is
needed for tinygo: replace `sync.Map.CompareAndDelete` in
[runcache.go](../internal/lint/runcache.go) with a mutex-guarded
map (the tinygo lever). Then `tinygo build -target wasm
./cmd/mdsmith-wasm` is reachable. `go.mod` sheds ~95 packages
plus apd and protobuf; `Capabilities()` is unchanged.

## Tasks

Split into per-phase plans, run in order, each keeping
`go test ./...` green; CUE left `go.mod` at the end (plan 240):

1. [Phase 0 — package, façade, harness](236_cuelite-package-harness.md)
2. [Phase 1 — surface D (placeholder paths)](237_cuelite-surface-d.md)
3. [Phase 2 — surfaces A + B (schema, query)](238_cuelite-surfaces-ab.md)
4. [Phase 3 — surface C (row-expr evaluator)](239_cuelite-surface-c.md)
5. [Phase 4 — drop cuelang.org + tinygo](240_cuelite-drop-cue.md)

## Acceptance Criteria

- [x] `cuelang.org/go` no longer appears in `go.mod` or
      `go.sum`, and no file (test or non-test) imports
      `cuelang.org/...` (plan 240).
- [x] The differential harness showed `cue/cuelite` and
      CUE agree on accept/reject and error field-paths, over
      the full corpus, the conflict table, and the query
      examples — verified GREEN before CUE was removed, then
      the harness was deleted with its corpus ported to
      engine-only pinned tests (plan 240).
- [x] `go run ./cmd/mdsmith check .` passes unchanged: every
      existing schema, `proto.md`, plan, and query stays valid,
      with no syntax migration.
- [x] `cue/cuelite` validation skips the per-check JSON
      round-trip (CompileMap); an absolute allocs/op guard
      replaced the CUE-relative factor gate (`bench_test.go`).
- [x] A benchmark records the validate and row-render paths;
      validate stays within its alloc ceiling.
- [x] `cue/cuelite` is a documented, exported public package.
- [x] Standard-Go WASM artifact ≤ 18 MB (measured ~11.2 MB).
- [🔳] `tinygo build -target wasm` succeeds and is ≤ 8 MB —
      blocked by the os.Chmod / os.SameFile / os.Symlink gaps,
      scheduled as [plan 247](247_tinygo-wasm-build.md).
- [x] MDS020 diagnostics stay actionable and navigable (plan
      147 / 230 preserved; the schema engine is unchanged).
- [x] `cue/cuelite` reaches 100 % coverage — the engine logic and
      the `syntax` package's parser/scanner error branches are
      covered (`closeout_coverage_test.go`).
- [x] All tests pass: `go test ./...`
- [🔳] `go tool golangci-lint run` reports no issues — CI-verified
      (needs Go ≥ 1.25.8; the dev container has 1.25.0).

## Non-Goals

- The schema-unification YAML DSL
  ([Direction B](../docs/research/schema-unification/spike.md)):
  this keeps CUE syntax, it does not replace it.
- User-facing CUE `import` / `package` / `#def` syntax — never
  shipped, still out of scope.
- CUE for body/section structure: already a YAML + RE2 surface
  ([section-schema](../docs/reference/section-schema.md)),
  untouched here.
- Any change to the public
  [pkg/mdsmith](../docs/background/concepts/engine-api.md) API
  or to `Capabilities()`.

## See also

- [Plan 215: engine API and WASM bindings](215_engine-api-wasm.md)
- [Named field-type shortcuts](148_named-field-type-shortcuts.md)
